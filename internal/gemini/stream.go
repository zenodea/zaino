package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/sse"
)

type blockKind int

const (
	blockNone blockKind = iota
	blockText
	blockThinking
)

type stream struct {
	src     *sse.Reader
	closer  io.Closer
	pending []llm.Event
	current llm.Event
	err     error
	done    bool
	acc     llm.Accumulator

	startEmitted bool
	endEmitted   bool
	openKind     blockKind
	openIndex    int
	nextIndex    int
	callSeq      int

	sawToolCall  bool
	finishReason string
	blockReason  string
	usage        llm.Usage
}

func newStream(body io.ReadCloser) *stream {
	return &stream{src: sse.NewReader(body), closer: body, openIndex: -1}
}

func (s *stream) Event() llm.Event       { return s.current }
func (s *stream) Err() error             { return s.err }
func (s *stream) Close() error           { return s.closer.Close() }
func (s *stream) Message() *llm.Response { return s.acc.Response() }

func (s *stream) Next() bool {
	for {
		if len(s.pending) > 0 {
			ev := s.pending[0]
			s.pending = s.pending[1:]
			s.acc.Add(ev)
			s.current = ev
			return true
		}
		if s.done {
			return false
		}

		data, err := s.src.Next()
		if err != nil {
			s.done = true
			if !errors.Is(err, io.EOF) {
				s.err = err
				return false
			}

			if s.finishReason == "" && s.blockReason == "" {
				s.err = fmt.Errorf("gemini: stream ended without a finish reason: %w", io.ErrUnexpectedEOF)
				return false
			}
			if failure := s.fatalFinishReason(); failure != nil {
				s.err = failure
				return false
			}
			s.pending = s.endOfStreamEvents()
			continue
		}

		var chunk generateResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			s.done = true
			s.err = fmt.Errorf("gemini: malformed stream chunk: %w", err)
			return false
		}
		s.pending = append(s.pending, s.translateChunk(chunk)...)
	}
}

func (s *stream) translateChunk(chunk generateResponse) []llm.Event {
	var events []llm.Event

	if !s.startEmitted {
		s.startEmitted = true
		events = append(events, llm.MessageStartEvent{Message: llm.Response{
			ID:      chunk.ResponseID,
			Model:   chunk.ModelVersion,
			Role:    llm.RoleAssistant,
			Content: llm.Content{},
		}})
	}
	if chunk.PromptFeedback != nil && chunk.PromptFeedback.BlockReason != "" {
		s.blockReason = chunk.PromptFeedback.BlockReason
	}
	if chunk.UsageMetadata != nil {
		s.usage = chunk.UsageMetadata.toLLM()
	}

	for _, cand := range chunk.Candidates {
		if cand.Index != 0 {
			continue
		}
		for _, p := range cand.Content.Parts {
			events = append(events, s.translatePart(p)...)
		}
		if cand.FinishReason != "" {
			s.finishReason = cand.FinishReason
		}
	}
	return events
}

func (s *stream) translatePart(p part) []llm.Event {
	switch {
	case p.FunctionCall != nil:
		var events []llm.Event
		events = append(events, s.closeOpenBlock()...)

		input, err := json.Marshal(p.FunctionCall.Args)
		if err != nil || len(p.FunctionCall.Args) == 0 {
			input = []byte("{}")
		}
		id := p.FunctionCall.ID
		if id == "" {
			id = fmt.Sprintf("%s%d-%s", syntheticIDPrefix, s.callSeq, p.FunctionCall.Name)
			s.callSeq++
		}

		index := s.nextIndex
		s.nextIndex++
		s.sawToolCall = true

		events = append(events,
			llm.ContentBlockStartEvent{Index: index, Block: llm.ToolUseBlock{
				ID:    id,
				Name:  p.FunctionCall.Name,
				Input: input,
			}},
			llm.ContentBlockStopEvent{Index: index},
		)
		return events

	case p.Thought:
		events := s.ensureOpenBlock(blockThinking)
		if p.Text != "" {
			events = append(events, llm.ContentBlockDeltaEvent{
				Index: s.openIndex,
				Delta: llm.ThinkingDelta{Thinking: p.Text},
			})
		}
		if p.ThoughtSignature != "" {
			events = append(events, llm.ContentBlockDeltaEvent{
				Index: s.openIndex,
				Delta: llm.SignatureDelta{Signature: p.ThoughtSignature},
			})
		}
		return events

	case p.Text != "":
		events := s.ensureOpenBlock(blockText)
		return append(events, llm.ContentBlockDeltaEvent{
			Index: s.openIndex,
			Delta: llm.TextDelta{Text: p.Text},
		})
	}
	return nil
}

func (s *stream) ensureOpenBlock(kind blockKind) []llm.Event {
	if s.openKind == kind {
		return nil
	}
	events := s.closeOpenBlock()

	index := s.nextIndex
	s.nextIndex++
	s.openKind = kind
	s.openIndex = index

	var block llm.ContentBlock = llm.TextBlock{}
	if kind == blockThinking {
		block = llm.ThinkingBlock{}
	}
	return append(events, llm.ContentBlockStartEvent{Index: index, Block: block})
}

func (s *stream) closeOpenBlock() []llm.Event {
	if s.openKind == blockNone {
		return nil
	}
	index := s.openIndex
	s.openKind = blockNone
	s.openIndex = -1
	return []llm.Event{llm.ContentBlockStopEvent{Index: index}}
}

// Gemini has no block or message terminators, so they are synthesised here.
func (s *stream) endOfStreamEvents() []llm.Event {
	if s.endEmitted {
		return nil
	}
	s.endEmitted = true

	events := s.closeOpenBlock()
	stop, details := s.stopReason()
	return append(events,
		llm.MessageDeltaEvent{StopReason: stop, StopDetails: details, Usage: s.usage},
		llm.MessageStopEvent{},
	)
}

func (s *stream) stopReason() (llm.StopReason, *llm.StopDetails) {
	if s.blockReason != "" {
		return llm.StopRefusal, &llm.StopDetails{
			Category:    s.blockReason,
			Explanation: "prompt blocked before generation",
		}
	}

	switch s.finishReason {
	case "MAX_TOKENS":
		return llm.StopMaxTokens, nil
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
		return llm.StopRefusal, &llm.StopDetails{
			Category:    s.finishReason,
			Explanation: "generation stopped by a Gemini safety filter",
		}
	}

	if s.sawToolCall {
		return llm.StopToolUse, nil
	}
	return llm.StopEndTurn, nil
}

func (s *stream) fatalFinishReason() error {
	switch s.finishReason {
	case "MALFORMED_FUNCTION_CALL":
		return errors.New("gemini: model emitted a malformed function call")
	case "UNEXPECTED_TOOL_CALL":
		return errors.New("gemini: model called a tool that was not declared")
	}
	return nil
}
