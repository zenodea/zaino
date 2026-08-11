package llm

import (
	"encoding/json"
	"fmt"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type StopReason string

const (
	StopEndTurn      StopReason = "end_turn"
	StopMaxTokens    StopReason = "max_tokens"
	StopStopSequence StopReason = "stop_sequence"
	StopToolUse      StopReason = "tool_use"
	StopPauseTurn    StopReason = "pause_turn"
	StopRefusal      StopReason = "refusal"
)

const (
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"
	EffortMax    = "max"
)

type ContentBlock interface {
	blockType() string
}

type TextBlock struct {
	Text string `json:"text"`
}

func (TextBlock) blockType() string { return "text" }

func (b TextBlock) MarshalJSON() ([]byte, error) {
	type alias TextBlock
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"text", alias(b)})
}

// Anthropic rejects thinking blocks that were modified on replay.
type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature,omitempty"`
}

func (ThinkingBlock) blockType() string { return "thinking" }

func (b ThinkingBlock) MarshalJSON() ([]byte, error) {
	type alias ThinkingBlock
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"thinking", alias(b)})
}

type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// Gemini rejects the next turn if the signature it put on a function call
	// does not come back. Anthropic has no equivalent and 400s on it.
	Signature string `json:"signature,omitempty"`
}

func (ToolUseBlock) blockType() string { return "tool_use" }

func (b ToolUseBlock) MarshalJSON() ([]byte, error) {
	if len(b.Input) == 0 {
		b.Input = json.RawMessage("{}")
	}
	type alias ToolUseBlock
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"tool_use", alias(b)})
}

type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (ToolResultBlock) blockType() string { return "tool_result" }

func (b ToolResultBlock) MarshalJSON() ([]byte, error) {
	type alias ToolResultBlock
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"tool_result", alias(b)})
}

type OpaqueBlock struct {
	Type string
	Raw  json.RawMessage
}

func (b OpaqueBlock) blockType() string { return b.Type }

func (b OpaqueBlock) MarshalJSON() ([]byte, error) { return b.Raw, nil }

type Content []ContentBlock

func (c *Content) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("content: %w", err)
	}
	out := make(Content, 0, len(raws))
	for i, raw := range raws {
		block, err := UnmarshalBlock(raw)
		if err != nil {
			return fmt.Errorf("content[%d]: %w", i, err)
		}
		out = append(out, block)
	}
	*c = out
	return nil
}

func UnmarshalBlock(raw json.RawMessage) (ContentBlock, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	switch probe.Type {
	case "text":
		var b TextBlock
		err := json.Unmarshal(raw, &b)
		return b, err
	case "thinking":
		var b ThinkingBlock
		err := json.Unmarshal(raw, &b)
		return b, err
	case "tool_use":
		var b ToolUseBlock
		err := json.Unmarshal(raw, &b)
		return b, err
	case "tool_result":
		var b ToolResultBlock
		err := json.Unmarshal(raw, &b)
		return b, err
	default:
		return OpaqueBlock{Type: probe.Type, Raw: append(json.RawMessage(nil), raw...)}, nil
	}
}

type Message struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

func UserText(text string) Message {
	return Message{Role: RoleUser, Content: Content{TextBlock{Text: text}}}
}

func (m Message) Text() string {
	var s string
	for _, b := range m.Content {
		if t, ok := b.(TextBlock); ok {
			s += t.Text
		}
	}
	return s
}

func (m Message) ToolUses() []ToolUseBlock {
	var out []ToolUseBlock
	for _, b := range m.Content {
		if t, ok := b.(ToolUseBlock); ok {
			out = append(out, t)
		}
	}
	return out
}

type Thinking struct {
	Enabled bool

	Budget int

	Show bool
}

type Tool struct {
	Name        string
	Description string

	InputSchema map[string]any
}

type Request struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []Tool
	Thinking  *Thinking

	Effort string
}

type Usage struct {
	InputTokens      int `json:"input,omitempty"`
	OutputTokens     int `json:"output,omitempty"`
	ThinkingTokens   int `json:"thinking,omitempty"`
	CacheReadTokens  int `json:"cache_read,omitempty"`
	CacheWriteTokens int `json:"cache_write,omitempty"`
}

type StopDetails struct {
	Category    string
	Explanation string
}

type Response struct {
	ID          string
	Model       string
	Role        Role
	Content     Content
	StopReason  StopReason
	StopDetails *StopDetails
	Usage       Usage
}

func (r *Response) ToMessage() Message {
	role := r.Role
	if role == "" {
		role = RoleAssistant
	}
	return Message{Role: role, Content: r.Content}
}

func (r *Response) Text() string { return r.ToMessage().Text() }

func (r *Response) ToolUses() []ToolUseBlock { return r.ToMessage().ToolUses() }
