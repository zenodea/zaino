package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/zenodea/zaino/internal/llm"
)

const (
	Model25Pro   = "gemini-2.5-pro"
	Model25Flash = "gemini-2.5-flash"
)

// Gemini omits call IDs on some models and matches responses by name, so
// IDs carrying this prefix are ours and are never sent back.
const syntheticIDPrefix = "zn-"

type functionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type functionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type functionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolSet struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type thinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
}

type generationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type generateRequest struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	Tools             []toolSet         `json:"tools,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

func (u usageMetadata) toLLM() llm.Usage {
	return llm.Usage{
		InputTokens:     u.PromptTokenCount,
		OutputTokens:    u.CandidatesTokenCount,
		ThinkingTokens:  u.ThoughtsTokenCount,
		CacheReadTokens: u.CachedContentTokenCount,
	}
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason"`
	Index        int     `json:"index"`
}

type generateResponse struct {
	Candidates     []candidate    `json:"candidates"`
	UsageMetadata  *usageMetadata `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	ModelVersion string `json:"modelVersion"`
	ResponseID   string `json:"responseId"`
}

func buildRequest(req llm.Request, defaultMaxTokens int) (generateRequest, error) {
	out := generateRequest{
		GenerationConfig: &generationConfig{
			MaxOutputTokens: orDefault(req.MaxTokens, defaultMaxTokens),
		},
	}

	if req.System != "" {
		out.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}

	names := toolCallNames(req.Messages)

	for _, msg := range req.Messages {
		c, err := translateMessage(msg, names)
		if err != nil {
			return out, err
		}
		if len(c.Parts) > 0 {
			out.Contents = append(out.Contents, c)
		}
	}

	if len(req.Tools) > 0 {
		decls := make([]functionDeclaration, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, functionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  sanitizeSchema(t.InputSchema),
			})
		}
		out.Tools = []toolSet{{FunctionDeclarations: decls}}
	}

	// Omitted rather than zero-budget when off: Pro models reject a zero budget.
	if t := req.Thinking; t != nil && t.Enabled {
		cfg := &thinkingConfig{IncludeThoughts: t.Show}
		if t.Budget > 0 {
			cfg.ThinkingBudget = &t.Budget
		}
		out.GenerationConfig.ThinkingConfig = cfg
	}

	return out, nil
}

func toolCallNames(messages []llm.Message) map[string]string {
	names := map[string]string{}
	for _, msg := range messages {
		for _, block := range msg.Content {
			if use, ok := block.(llm.ToolUseBlock); ok {
				names[use.ID] = use.Name
			}
		}
	}
	return names
}

func translateMessage(msg llm.Message, names map[string]string) (content, error) {
	out := content{Role: "user"}
	if msg.Role == llm.RoleAssistant {
		out.Role = "model"
	}

	for _, block := range msg.Content {
		switch b := block.(type) {
		case llm.TextBlock:
			if b.Text != "" {
				out.Parts = append(out.Parts, part{Text: b.Text})
			}

		case llm.ThinkingBlock:

		case llm.ToolUseBlock:
			args := map[string]any{}
			if len(b.Input) > 0 {
				if err := json.Unmarshal(b.Input, &args); err != nil {
					return out, fmt.Errorf("gemini: tool call %s has non-object input: %w", b.Name, err)
				}
			}
			call := &functionCall{Name: b.Name, Args: args}
			if !isSynthetic(b.ID) {
				call.ID = b.ID
			}
			out.Parts = append(out.Parts, part{FunctionCall: call})

		case llm.ToolResultBlock:
			name, ok := names[b.ToolUseID]
			if !ok {
				return out, fmt.Errorf("gemini: tool result %s has no matching call in history", b.ToolUseID)
			}

			key := "output"
			if b.IsError {
				key = "error"
			}
			resp := &functionResponse{Name: name, Response: map[string]any{key: b.Content}}
			if !isSynthetic(b.ToolUseID) {
				resp.ID = b.ToolUseID
			}
			out.Parts = append(out.Parts, part{FunctionResponse: resp})

		case llm.OpaqueBlock:

		}
	}
	return out, nil
}

func isSynthetic(id string) bool {
	return len(id) >= len(syntheticIDPrefix) && id[:len(syntheticIDPrefix)] == syntheticIDPrefix
}

// Gemini 400s on any other JSON Schema keyword.
var supportedSchemaKeys = map[string]bool{
	"type": true, "format": true, "description": true, "nullable": true,
	"enum": true, "items": true, "properties": true, "required": true,
	"anyOf": true, "minItems": true, "maxItems": true, "title": true,
}

func sanitizeSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		if !supportedSchemaKeys[k] {
			continue
		}
		switch k {
		case "properties":
			if props, ok := v.(map[string]any); ok {
				cleaned := make(map[string]any, len(props))
				for name, sub := range props {
					cleaned[name] = sanitizeValue(sub)
				}
				out[k] = cleaned
				continue
			}
		case "items", "anyOf":
			out[k] = sanitizeValue(v)
			continue
		}
		out[k] = v
	}
	return out
}

func sanitizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return sanitizeSchema(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = sanitizeValue(item)
		}
		return out
	default:
		return v
	}
}

func orDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}
