package openaicompat

import (
	"fmt"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

type functionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type toolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function functionCall `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type functionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []message      `json:"messages"`
	Tools         []toolDef      `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`

	// Newer OpenAI models reject max_tokens and older compatible hosts
	// reject max_completion_tokens, so the field name comes from the vendor.
	MaxTokens       *int   `json:"max_tokens,omitempty"`
	MaxCompletion   *int   `json:"max_completion_tokens,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type usage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u usage) toLLM() llm.Usage {
	out := llm.Usage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens}
	if u.PromptTokensDetails != nil {
		out.CacheReadTokens = u.PromptTokensDetails.CachedTokens
		out.InputTokens = max(out.InputTokens-out.CacheReadTokens, 0)
	}
	if u.CompletionTokensDetails != nil {
		out.ThinkingTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return out
}

type delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Refusal string `json:"refusal,omitempty"`
	// Grok and other compatible hosts stream reasoning here; OpenAI does not.
	Reasoning string     `json:"reasoning_content,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type choice struct {
	Index        int    `json:"index"`
	Delta        delta  `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

type chatChunk struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

const roleTool = "tool"

func buildRequest(req llm.Request, cfg Config, defaultModel string) (chatRequest, error) {
	out := chatRequest{
		Model:         orString(req.Model, defaultModel),
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}

	if n := req.MaxTokens; n > 0 {
		if cfg.LegacyMaxTokens {
			out.MaxTokens = &n
		} else {
			out.MaxCompletion = &n
		}
	}
	if cfg.ReasoningEffort {
		out.ReasoningEffort = reasoningEffort(req.Effort)
	}

	if req.System != "" {
		out.Messages = append(out.Messages, message{Role: cfg.systemRole(), Content: req.System})
	}

	calls := toolCallIDs(req.Messages)
	for _, msg := range req.Messages {
		translated, err := translateMessage(msg, calls)
		if err != nil {
			return out, err
		}
		out.Messages = append(out.Messages, translated...)
	}

	for _, t := range req.Tools {
		out.Tools = append(out.Tools, toolDef{
			Type: "function",
			Function: functionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out, nil
}

// A tool result whose call is not in the history makes the whole request
// invalid, and the server's complaint about it is unreadable.
func toolCallIDs(messages []llm.Message) map[string]bool {
	ids := map[string]bool{}
	for _, msg := range messages {
		for _, block := range msg.Content {
			if use, ok := block.(llm.ToolUseBlock); ok {
				ids[use.ID] = true
			}
		}
	}
	return ids
}

func translateMessage(msg llm.Message, calls map[string]bool) ([]message, error) {
	var text strings.Builder
	var toolCalls []toolCall
	var results []message

	for _, block := range msg.Content {
		switch b := block.(type) {
		case llm.TextBlock:
			text.WriteString(b.Text)

		case llm.ToolUseBlock:
			input := string(b.Input)
			if input == "" {
				input = "{}"
			}
			toolCalls = append(toolCalls, toolCall{
				ID:       b.ID,
				Type:     "function",
				Function: functionCall{Name: b.Name, Arguments: input},
			})

		case llm.ToolResultBlock:
			if !calls[b.ToolUseID] {
				return nil, fmt.Errorf("openai: tool result %q has no matching call", b.ToolUseID)
			}
			results = append(results, message{
				Role:       roleTool,
				ToolCallID: b.ToolUseID,
				Content:    orString(b.Content, "(no output)"),
			})

		// Chat Completions has nowhere to put reasoning, and replaying it as
		// assistant text would have the model answer its own thinking.
		case llm.ThinkingBlock, llm.OpaqueBlock:
		}
	}

	// Results have to precede the turn that reads them.
	out := results
	if text.Len() > 0 || len(toolCalls) > 0 {
		out = append(out, message{
			Role:      string(orRole(msg.Role)),
			Content:   text.String(),
			ToolCalls: toolCalls,
		})
	}
	return out, nil
}

func reasoningEffort(effort string) string {
	switch effort {
	case llm.EffortLow:
		return "low"
	case llm.EffortMedium:
		return "medium"
	case llm.EffortHigh, llm.EffortXHigh, llm.EffortMax:
		return "high"
	}
	return ""
}

func stopReason(finish string, sawTool bool) (llm.StopReason, *llm.StopDetails) {
	switch finish {
	case "length":
		return llm.StopMaxTokens, nil
	case "tool_calls", "function_call":
		return llm.StopToolUse, nil
	case "content_filter":
		return llm.StopRefusal, &llm.StopDetails{
			Category:    "content_filter",
			Explanation: "generation stopped by a content filter",
		}
	}
	if sawTool {
		return llm.StopToolUse, nil
	}
	return llm.StopEndTurn, nil
}

func orString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func orRole(r llm.Role) llm.Role {
	if r == "" {
		return llm.RoleUser
	}
	return r
}
