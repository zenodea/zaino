package llm

import (
	"bytes"
	"encoding/json"
)

type Accumulator struct {
	resp Response

	partial map[int]*bytes.Buffer
	stopped bool
}

func (a *Accumulator) Add(ev Event) {
	switch e := ev.(type) {
	case MessageStartEvent:
		a.resp = e.Message
		if a.resp.Content == nil {
			a.resp.Content = Content{}
		}

	case ContentBlockStartEvent:
		a.growContentTo(e.Index)
		a.resp.Content[e.Index] = e.Block
		if _, ok := e.Block.(ToolUseBlock); ok {
			if a.partial == nil {
				a.partial = map[int]*bytes.Buffer{}
			}
			a.partial[e.Index] = &bytes.Buffer{}
		}

	case ContentBlockDeltaEvent:
		a.growContentTo(e.Index)
		a.applyDelta(e.Index, e.Delta)

	case ContentBlockStopEvent:
		buf, ok := a.partial[e.Index]
		if !ok {
			return
		}
		if block, ok := a.resp.Content[e.Index].(ToolUseBlock); ok && buf.Len() > 0 {
			block.Input = json.RawMessage(append([]byte(nil), buf.Bytes()...))
			a.resp.Content[e.Index] = block
		}
		delete(a.partial, e.Index)

	case MessageDeltaEvent:
		if e.StopReason != "" {
			a.resp.StopReason = e.StopReason
		}
		if e.StopDetails != nil {
			a.resp.StopDetails = e.StopDetails
		}
		a.mergeMaxUsage(e.Usage)

	case MessageStopEvent:
		a.stopped = true
	}
}

func (a *Accumulator) Stopped() bool { return a.stopped }

func (a *Accumulator) Response() *Response { return &a.resp }

func (a *Accumulator) applyDelta(index int, delta Delta) {
	switch d := delta.(type) {
	case TextDelta:
		block, ok := a.resp.Content[index].(TextBlock)
		if !ok {
			return
		}
		block.Text += d.Text
		a.resp.Content[index] = block

	case ThinkingDelta:
		block, ok := a.resp.Content[index].(ThinkingBlock)
		if !ok {
			return
		}
		block.Thinking += d.Thinking
		a.resp.Content[index] = block

	case SignatureDelta:
		block, ok := a.resp.Content[index].(ThinkingBlock)
		if !ok {
			return
		}
		block.Signature += d.Signature
		a.resp.Content[index] = block

	case InputJSONDelta:
		if buf, ok := a.partial[index]; ok {
			buf.WriteString(d.PartialJSON)
		}
	}
}

// Max, not sum: Gemini repeats cumulative usage on every chunk.
func (a *Accumulator) mergeMaxUsage(u Usage) {
	a.resp.Usage.InputTokens = max(a.resp.Usage.InputTokens, u.InputTokens)
	a.resp.Usage.OutputTokens = max(a.resp.Usage.OutputTokens, u.OutputTokens)
	a.resp.Usage.ThinkingTokens = max(a.resp.Usage.ThinkingTokens, u.ThinkingTokens)
	a.resp.Usage.CacheReadTokens = max(a.resp.Usage.CacheReadTokens, u.CacheReadTokens)
	a.resp.Usage.CacheWriteTokens = max(a.resp.Usage.CacheWriteTokens, u.CacheWriteTokens)
}

func (a *Accumulator) growContentTo(i int) {
	for len(a.resp.Content) <= i {
		a.resp.Content = append(a.resp.Content, nil)
	}
}
