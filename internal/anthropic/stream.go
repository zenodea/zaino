package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/sse"
)

type stream struct {
	src     *sse.Reader
	closer  io.Closer
	current llm.Event
	err     error
	done    bool
	acc     llm.Accumulator
}

func newStream(body io.ReadCloser) *stream {
	return &stream{src: sse.NewReader(body), closer: body}
}

func (s *stream) Event() llm.Event       { return s.current }
func (s *stream) Err() error             { return s.err }
func (s *stream) Close() error           { return s.closer.Close() }
func (s *stream) Message() *llm.Response { return s.acc.Response() }

func (s *stream) Next() bool {
	if s.done {
		return false
	}
	for {
		data, err := s.src.Next()
		if err != nil {
			s.done = true
			if errors.Is(err, io.EOF) {
				if !s.acc.Stopped() {
					s.err = fmt.Errorf("anthropic: stream ended before message_stop: %w", io.ErrUnexpectedEOF)
				}
				return false
			}
			s.err = err
			return false
		}

		ev, err := decodeEvent(data)
		if err != nil {
			s.done = true
			s.err = err
			return false
		}
		if ev == nil {
			continue
		}

		s.acc.Add(ev)
		s.current = ev
		return true
	}
}

func decodeEvent(data []byte) (llm.Event, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("anthropic: malformed stream event: %w", err)
	}

	switch probe.Type {
	case "message_start":
		var payload struct {
			Message wireResponse `json:"message"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		return llm.MessageStartEvent{Message: payload.Message.toLLM()}, nil

	case "content_block_start":
		var payload struct {
			Index int             `json:"index"`
			Block json.RawMessage `json:"content_block"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		block, err := llm.UnmarshalBlock(payload.Block)
		if err != nil {
			return nil, err
		}
		return llm.ContentBlockStartEvent{Index: payload.Index, Block: block}, nil

	case "content_block_delta":
		var payload struct {
			Index int             `json:"index"`
			Delta json.RawMessage `json:"delta"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		delta, err := decodeDelta(payload.Delta)
		if err != nil {
			return nil, err
		}
		return llm.ContentBlockDeltaEvent{Index: payload.Index, Delta: delta}, nil

	case "content_block_stop":
		var payload struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		return llm.ContentBlockStopEvent{Index: payload.Index}, nil

	case "message_delta":
		var payload struct {
			Delta struct {
				StopReason  llm.StopReason   `json:"stop_reason"`
				StopDetails *wireStopDetails `json:"stop_details"`
			} `json:"delta"`
			Usage wireUsage `json:"usage"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		return llm.MessageDeltaEvent{
			StopReason:  payload.Delta.StopReason,
			StopDetails: payload.Delta.StopDetails.toLLM(),
			Usage:       payload.Usage.toLLM(),
		}, nil

	case "message_stop":
		return llm.MessageStopEvent{}, nil

	case "ping":
		return llm.PingEvent{}, nil

	case "error":
		var payload struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		return nil, &APIError{
			Type:    payload.Error.Type,
			Message: payload.Error.Message,
			Body:    append([]byte(nil), data...),
		}

	default:
		return nil, nil
	}
}

func decodeDelta(raw json.RawMessage) (llm.Delta, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	switch probe.Type {
	case "text_delta":
		var d struct {
			Text string `json:"text"`
		}
		err := json.Unmarshal(raw, &d)
		return llm.TextDelta{Text: d.Text}, err
	case "thinking_delta":
		var d struct {
			Thinking string `json:"thinking"`
		}
		err := json.Unmarshal(raw, &d)
		return llm.ThinkingDelta{Thinking: d.Thinking}, err
	case "signature_delta":
		var d struct {
			Signature string `json:"signature"`
		}
		err := json.Unmarshal(raw, &d)
		return llm.SignatureDelta{Signature: d.Signature}, err
	case "input_json_delta":
		var d struct {
			PartialJSON string `json:"partial_json"`
		}
		err := json.Unmarshal(raw, &d)
		return llm.InputJSONDelta{PartialJSON: d.PartialJSON}, err
	default:
		return llm.OpaqueDelta{Type: probe.Type, Raw: append(json.RawMessage(nil), raw...)}, nil
	}
}
