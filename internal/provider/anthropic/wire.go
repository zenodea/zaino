package anthropic

import (
	"encoding/json"
	"slices"

	"github.com/zenodea/zaino/internal/llm"
)

const (
	ModelFable5   = "claude-fable-5"
	ModelOpus5    = "claude-opus-5"
	ModelOpus48   = "claude-opus-4-8"
	ModelOpus47   = "claude-opus-4-7"
	ModelOpus46   = "claude-opus-4-6"
	ModelSonnet5  = "claude-sonnet-5"
	ModelSonnet46 = "claude-sonnet-4-6"
	ModelHaiku45  = "claude-haiku-4-5"
)

// Copy-on-write: the caller's history keeps the signature, in case the
// conversation goes back to the provider that issued it.
func stripToolSignatures(messages []llm.Message) []llm.Message {
	if !slices.ContainsFunc(messages, hasToolSignature) {
		return messages
	}

	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for i, m := range out {
		if !hasToolSignature(m) {
			continue
		}
		content := make(llm.Content, len(m.Content))
		copy(content, m.Content)
		for j, block := range content {
			if use, ok := block.(llm.ToolUseBlock); ok {
				use.Signature = ""
				content[j] = use
			}
		}
		out[i].Content = content
	}
	return out
}

func hasToolSignature(m llm.Message) bool {
	return slices.ContainsFunc(m.Content, func(block llm.ContentBlock) bool {
		use, ok := block.(llm.ToolUseBlock)
		return ok && use.Signature != ""
	})
}

type wireCacheControl struct {
	Type string `json:"type"`
}

// The conversation as it goes out, with the cache breakpoints marked. The
// blocks still encode themselves; this only says where a kept prefix ends.
type wireMessages struct {
	messages []llm.Message
	marked   map[int]bool
}

// Anthropic keeps the prefix up to each breakpoint and reads back the longest
// one that still matches. The turn just finished is marked so the next turn
// reads it back, and the end of the previous turn is marked too: that prefix
// was written last time, so marking it again is a read rather than a second
// write, and it survives a turn that added a whole batch of tool results.
func cacheMessages(messages []llm.Message) wireMessages {
	marked := map[int]bool{}
	if last := len(messages) - 1; last >= 0 {
		marked[last] = true
		for i := last - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				marked[i] = true
				break
			}
		}
	}
	return wireMessages{messages: messages, marked: marked}
}

func (m wireMessages) MarshalJSON() ([]byte, error) {
	out := make([]json.RawMessage, len(m.messages))
	for i, message := range m.messages {
		content := make([]json.RawMessage, len(message.Content))
		for j, block := range message.Content {
			raw, err := json.Marshal(block)
			if err != nil {
				return nil, err
			}
			if m.marked[i] && j == len(message.Content)-1 {
				if raw, err = withCacheControl(raw); err != nil {
					return nil, err
				}
			}
			content[j] = raw
		}

		raw, err := json.Marshal(struct {
			Role    llm.Role          `json:"role"`
			Content []json.RawMessage `json:"content"`
		}{message.Role, content})
		if err != nil {
			return nil, err
		}
		out[i] = raw
	}
	return json.Marshal(out)
}

// Spliced into a block that has already encoded itself, so no block type has
// to know about caching.
func withCacheControl(raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		// Not an object, so there is nothing to mark. A block that cannot
		// carry the marker is not worth failing the request over.
		return raw, nil
	}
	fields["cache_control"] = json.RawMessage(`{"type":"ephemeral"}`)
	return json.Marshal(fields)
}

type wireSystemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

type wireThinking struct {
	Type    string `json:"type"`
	Display string `json:"display,omitempty"`
}

type wireOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type wireRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`

	Messages     wireMessages      `json:"messages"`
	System       []wireSystemBlock `json:"system,omitempty"`
	Tools        []wireTool        `json:"tools,omitempty"`
	Thinking     *wireThinking     `json:"thinking,omitempty"`
	OutputConfig *wireOutputConfig `json:"output_config,omitempty"`
	Stream       bool              `json:"stream"`
}

// /v1/messages/count_tokens rejects the parameters that shape the reply, so it
// gets the same request with those left off.
type wireCountRequest struct {
	Model    string            `json:"model"`
	Messages wireMessages      `json:"messages"`
	System   []wireSystemBlock `json:"system,omitempty"`
	Tools    []wireTool        `json:"tools,omitempty"`
	Thinking *wireThinking     `json:"thinking,omitempty"`
}

func buildCountRequest(req llm.Request, defaultModel string) wireCountRequest {
	full := buildRequest(req, defaultModel)
	return wireCountRequest{
		Model:    full.Model,
		Messages: full.Messages,
		System:   full.System,
		Tools:    full.Tools,
		Thinking: full.Thinking,
	}
}

func buildRequest(req llm.Request, defaultModel string) wireRequest {
	out := wireRequest{
		Model:     orDefault(req.Model, defaultModel),
		MaxTokens: orDefault(req.MaxTokens, 8192),
		Messages:  cacheMessages(stripToolSignatures(req.Messages)),
		Stream:    true,
	}

	if req.System != "" {
		out.System = []wireSystemBlock{{
			Type:         "text",
			Text:         req.System,
			CacheControl: &wireCacheControl{Type: "ephemeral"},
		}}
	}

	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	if req.Effort != "" {
		out.OutputConfig = &wireOutputConfig{Effort: req.Effort}
	}

	if t := req.Thinking; t != nil {
		if t.Enabled {
			out.Thinking = &wireThinking{Type: "adaptive", Display: "omitted"}
			if t.Show {
				out.Thinking.Display = "summarized"
			}
		} else {
			// Disabling thinking is rejected above "high" effort.
			switch req.Effort {
			case llm.EffortXHigh, llm.EffortMax:
			default:
				out.Thinking = &wireThinking{Type: "disabled"}
			}
		}
	}
	return out
}

type wireUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u wireUsage) toLLM() llm.Usage {
	return llm.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

type wireStopDetails struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
}

func (d *wireStopDetails) toLLM() *llm.StopDetails {
	if d == nil {
		return nil
	}
	return &llm.StopDetails{Category: d.Category, Explanation: d.Explanation}
}

type wireResponse struct {
	ID          string           `json:"id"`
	Model       string           `json:"model"`
	Role        llm.Role         `json:"role"`
	Content     llm.Content      `json:"content"`
	StopReason  llm.StopReason   `json:"stop_reason"`
	StopDetails *wireStopDetails `json:"stop_details"`
	Usage       wireUsage        `json:"usage"`
}

func (r wireResponse) toLLM() llm.Response {
	return llm.Response{
		ID:          r.ID,
		Model:       r.Model,
		Role:        r.Role,
		Content:     r.Content,
		StopReason:  r.StopReason,
		StopDetails: r.StopDetails.toLLM(),
		Usage:       r.Usage.toLLM(),
	}
}

func orDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}
