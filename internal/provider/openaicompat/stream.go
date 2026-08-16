package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/x/sse"
)

type blockKind int

const (
	blockNone blockKind = iota
	blockText
	blockThinking
)

const doneMarker = "[DONE]"

type stream struct {
	provider string
	src      *sse.Reader
	closer   io.Closer

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

	// The wire numbers tool calls within a turn; content blocks are numbered
	// across the whole message, so the two have to be kept apart.
	toolBlocks map[int]int
	toolOrder  []int

	sawToolCall  bool
	finishReason string
	refusal      string
	usage        llm.Usage
}

func newStream(provider string, body io.ReadCloser) *stream {
	return &stream{
		provider:   provider,
		src:        sse.NewReader(body),
		closer:     body,
		openIndex:  -1,
		toolBlocks: map[int]int{},
	}
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
			if !s.endEmitted && s.finishReason == "" {
				s.err = fmt.Errorf("%s: stream ended without a finish reason: %w",
					s.provider, io.ErrUnexpectedEOF)
				return false
			}
			s.pending = s.endOfStreamEvents()
			continue
		}

		if string(data) == doneMarker {
			s.done = true
			if !s.startEmitted {
				s.err = fmt.Errorf("%s: stream carried no content: %w",
					s.provider, io.ErrUnexpectedEOF)
				return false
			}
			s.pending = s.endOfStreamEvents()
			continue
		}

		var chunk chatChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			s.done = true
			s.err = fmt.Errorf("%s: malformed stream chunk: %w", s.provider, err)
			return false
		}
		if chunk.Error != nil {
			s.done = true
			s.err = &APIError{
				Provider: s.provider,
				Type:     orString(chunk.Error.Type, "stream_error"),
				Message:  chunk.Error.Message,
			}
			return false
		}
		s.pending = append(s.pending, s.translateChunk(chunk)...)
	}
}

func (s *stream) translateChunk(chunk chatChunk) []llm.Event {
	var events []llm.Event

	if !s.startEmitted {
		s.startEmitted = true
		events = append(events, llm.MessageStartEvent{Message: llm.Response{
			ID:      chunk.ID,
			Model:   chunk.Model,
			Role:    llm.RoleAssistant,
			Content: llm.Content{},
		}})
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage.toLLM()
	}

	for _, ch := range chunk.Choices {
		if ch.Index != 0 {
			continue
		}
		events = append(events, s.translateDelta(ch.Delta)...)
		if ch.FinishReason != "" {
			s.finishReason = ch.FinishReason
		}
	}
	return events
}

func (s *stream) translateDelta(d delta) []llm.Event {
	var events []llm.Event

	if d.Reasoning != "" {
		events = append(events, s.ensureOpenBlock(blockThinking)...)
		events = append(events, llm.ContentBlockDeltaEvent{
			Index: s.openIndex,
			Delta: llm.ThinkingDelta{Thinking: d.Reasoning},
		})
	}
	if d.Content != "" {
		events = append(events, s.ensureOpenBlock(blockText)...)
		events = append(events, llm.ContentBlockDeltaEvent{
			Index: s.openIndex,
			Delta: llm.TextDelta{Text: d.Content},
		})
	}
	if d.Refusal != "" {
		s.refusal += d.Refusal
	}

	for _, call := range d.ToolCalls {
		events = append(events, s.translateToolCall(call)...)
	}
	return events
}

func (s *stream) translateToolCall(call toolCall) []llm.Event {
	var events []llm.Event

	index, open := s.toolBlocks[call.Index]
	if !open {
		events = append(events, s.closeOpenBlock()...)

		index = s.nextIndex
		s.nextIndex++
		s.toolBlocks[call.Index] = index
		s.toolOrder = append(s.toolOrder, call.Index)
		s.sawToolCall = true

		events = append(events, llm.ContentBlockStartEvent{Index: index, Block: llm.ToolUseBlock{
			ID:   orString(call.ID, fmt.Sprintf("call_%d", call.Index)),
			Name: call.Function.Name,
		}})
	}

	if call.Function.Arguments != "" {
		events = append(events, llm.ContentBlockDeltaEvent{
			Index: index,
			Delta: llm.InputJSONDelta{PartialJSON: call.Function.Arguments},
		})
	}
	return events
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

// Chat Completions terminates neither blocks nor the message, so both are
// synthesised once the stream runs out.
func (s *stream) endOfStreamEvents() []llm.Event {
	if s.endEmitted {
		return nil
	}
	s.endEmitted = true

	events := s.closeOpenBlock()
	for _, wire := range s.toolOrder {
		events = append(events, llm.ContentBlockStopEvent{Index: s.toolBlocks[wire]})
	}

	stop, details := stopReason(s.finishReason, s.sawToolCall)
	if s.refusal != "" {
		stop, details = llm.StopRefusal, &llm.StopDetails{
			Category:    "refusal",
			Explanation: s.refusal,
		}
	}
	return append(events,
		llm.MessageDeltaEvent{StopReason: stop, StopDetails: details, Usage: s.usage},
		llm.MessageStopEvent{},
	)
}
