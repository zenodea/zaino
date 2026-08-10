package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func sseBody(chunks ...string) io.ReadCloser {
	return io.NopCloser(strings.NewReader("data: " + strings.Join(chunks, "\n\ndata: ") + "\n\n"))
}

func drain(t *testing.T, s *stream) *llm.Response {
	t.Helper()
	for s.Next() {
	}
	if err := s.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	return s.Message()
}

func TestStreamInfersBlocks(t *testing.T) {
	s := newStream(sseBody(
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"thinking about it","thought":true}]},"index":0}],"modelVersion":"gemini-2.5-pro","responseId":"resp_1"}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"Let me "}]},"index":0}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"check."}]},"index":0}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"/etc/hosts"}}}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":25,"candidatesTokenCount":57,"thoughtsTokenCount":8}}`,
	))
	defer s.Close()

	resp := drain(t, s)

	if resp.ID != "resp_1" || resp.Model != "gemini-2.5-pro" {
		t.Errorf("header not carried: %+v", resp)
	}
	if resp.Role != llm.RoleAssistant {
		t.Errorf("role = %q", resp.Role)
	}

	if resp.StopReason != llm.StopToolUse {
		t.Errorf("stop reason = %q, want tool_use", resp.StopReason)
	}
	if resp.Usage.InputTokens != 25 || resp.Usage.OutputTokens != 57 || resp.Usage.ThinkingTokens != 8 {
		t.Errorf("usage = %+v", resp.Usage)
	}

	if len(resp.Content) != 3 {
		t.Fatalf("got %d blocks, want 3 (thinking, text, tool_use): %+v", len(resp.Content), resp.Content)
	}
	think, ok := resp.Content[0].(llm.ThinkingBlock)
	if !ok || think.Thinking != "thinking about it" {
		t.Errorf("block 0 = %#v", resp.Content[0])
	}

	text, ok := resp.Content[1].(llm.TextBlock)
	if !ok || text.Text != "Let me check." {
		t.Errorf("block 1 = %#v", resp.Content[1])
	}
	use, ok := resp.Content[2].(llm.ToolUseBlock)
	if !ok {
		t.Fatalf("block 2 = %#v", resp.Content[2])
	}
	if use.Name != "read" {
		t.Errorf("tool name = %q", use.Name)
	}
	if got := string(use.Input); got != `{"path":"/etc/hosts"}` {
		t.Errorf("tool input = %s", got)
	}

	if !isSynthetic(use.ID) {
		t.Errorf("expected a synthetic id, got %q", use.ID)
	}
}

func TestStreamKeepsRealCallID(t *testing.T) {
	s := newStream(sseBody(
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_abc","name":"read","args":{}}}]},"finishReason":"STOP","index":0}]}`,
	))
	defer s.Close()

	use := drain(t, s).ToolUses()[0]
	if use.ID != "call_abc" {
		t.Errorf("id = %q, want call_abc", use.ID)
	}
	if got := string(use.Input); got != "{}" {
		t.Errorf("empty args should serialise as {}, got %s", got)
	}
}

func TestStreamStopReasons(t *testing.T) {
	cases := []struct {
		name  string
		chunk string
		want  llm.StopReason
	}{
		{"end turn", `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP","index":0}]}`, llm.StopEndTurn},
		{"truncated", `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"MAX_TOKENS","index":0}]}`, llm.StopMaxTokens},
		{"safety", `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"SAFETY","index":0}]}`, llm.StopRefusal},
		{"prompt blocked", `{"promptFeedback":{"blockReason":"SAFETY"}}`, llm.StopRefusal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStream(sseBody(tc.chunk))
			defer s.Close()
			resp := drain(t, s)
			if resp.StopReason != tc.want {
				t.Errorf("stop reason = %q, want %q", resp.StopReason, tc.want)
			}
			if tc.want == llm.StopRefusal && resp.StopDetails == nil {
				t.Error("refusal should carry StopDetails")
			}
		})
	}
}

func TestStreamMalformedFunctionCallIsError(t *testing.T) {
	s := newStream(sseBody(
		`{"candidates":[{"content":{"parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL","index":0}]}`,
	))
	defer s.Close()
	for s.Next() {
	}
	if s.Err() == nil {
		t.Fatal("want an error for MALFORMED_FUNCTION_CALL")
	}
}

func TestStreamTruncatedConnection(t *testing.T) {
	s := newStream(sseBody(
		`{"candidates":[{"content":{"parts":[{"text":"partial"}]},"index":0}]}`,
	))
	defer s.Close()
	for s.Next() {
	}
	if s.Err() == nil {
		t.Fatal("want a truncation error")
	}
	if got := s.Message().Text(); got != "partial" {
		t.Errorf("partial text should still accumulate, got %q", got)
	}
}

func TestBuildRequestResolvesToolResultNames(t *testing.T) {
	history := []llm.Message{
		llm.UserText("read the file"),
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ThinkingBlock{Thinking: "internal"},
			llm.ToolUseBlock{ID: "zn-0-read", Name: "read", Input: json.RawMessage(`{"path":"/tmp/x"}`)},
			llm.ToolUseBlock{ID: "call_real", Name: "stat", Input: json.RawMessage(`{}`)},
		}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "zn-0-read", Content: "file body"},
			llm.ToolResultBlock{ToolUseID: "call_real", Content: "boom", IsError: true},
		}},
	}

	req, err := buildRequest(llm.Request{Messages: history, System: "be brief"}, 4096)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	if req.SystemInstruction == nil || req.SystemInstruction.Parts[0].Text != "be brief" {
		t.Errorf("system instruction = %+v", req.SystemInstruction)
	}
	if len(req.Contents) != 3 {
		t.Fatalf("got %d contents, want 3", len(req.Contents))
	}
	if req.Contents[1].Role != "model" {
		t.Errorf("assistant role = %q, want model", req.Contents[1].Role)
	}

	assistant := req.Contents[1].Parts
	if len(assistant) != 2 {
		t.Fatalf("assistant parts = %d, want 2 (thinking dropped): %+v", len(assistant), assistant)
	}

	if assistant[0].FunctionCall.ID != "" {
		t.Errorf("synthetic id leaked to the wire: %q", assistant[0].FunctionCall.ID)
	}
	if assistant[1].FunctionCall.ID != "call_real" {
		t.Errorf("real id = %q, want call_real", assistant[1].FunctionCall.ID)
	}

	results := req.Contents[2].Parts
	if len(results) != 2 {
		t.Fatalf("result parts = %d, want 2", len(results))
	}
	if results[0].FunctionResponse.Name != "read" {
		t.Errorf("result 0 name = %q, want read (resolved from history)", results[0].FunctionResponse.Name)
	}
	if got := results[0].FunctionResponse.Response["output"]; got != "file body" {
		t.Errorf("success result should use the output key, got %+v", results[0].FunctionResponse.Response)
	}
	if got := results[1].FunctionResponse.Response["error"]; got != "boom" {
		t.Errorf("error result should use the error key, got %+v", results[1].FunctionResponse.Response)
	}
}

func TestBuildRequestRejectsOrphanToolResult(t *testing.T) {
	_, err := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleUser, Content: llm.Content{llm.ToolResultBlock{ToolUseID: "ghost"}}},
	}}, 4096)
	if err == nil {
		t.Fatal("want an error when a tool result has no matching call")
	}
}

func TestSanitizeSchemaStripsUnsupportedKeywords(t *testing.T) {
	got := sanitizeSchema(map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "file path",
				"minLength":   float64(1),
			},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "pattern": "^x"},
			},
		},
	})

	if _, ok := got["$schema"]; ok {
		t.Error("$schema not stripped")
	}
	if _, ok := got["additionalProperties"]; ok {
		t.Error("additionalProperties not stripped")
	}
	if got["type"] != "object" {
		t.Errorf("type = %v", got["type"])
	}
	props := got["properties"].(map[string]any)
	path := props["path"].(map[string]any)
	if _, ok := path["minLength"]; ok {
		t.Error("minLength not stripped from a nested property")
	}
	if path["description"] != "file path" {
		t.Error("description should survive")
	}
	items := props["tags"].(map[string]any)["items"].(map[string]any)
	if _, ok := items["pattern"]; ok {
		t.Error("pattern not stripped from items")
	}
}

func TestBuildRequestThinkingConfig(t *testing.T) {
	on, _ := buildRequest(llm.Request{Thinking: &llm.Thinking{Enabled: true, Show: true, Budget: 2048}}, 4096)
	cfg := on.GenerationConfig.ThinkingConfig
	if cfg == nil || !cfg.IncludeThoughts || cfg.ThinkingBudget == nil || *cfg.ThinkingBudget != 2048 {
		t.Errorf("thinking config = %+v", cfg)
	}

	off, _ := buildRequest(llm.Request{Thinking: &llm.Thinking{Enabled: false}}, 4096)
	if off.GenerationConfig.ThinkingConfig != nil {
		t.Errorf("thinking config should be omitted when disabled, got %+v",
			off.GenerationConfig.ThinkingConfig)
	}

	noBudget, _ := buildRequest(llm.Request{Thinking: &llm.Thinking{Enabled: true}}, 4096)
	if b := noBudget.GenerationConfig.ThinkingConfig.ThinkingBudget; b != nil {
		t.Errorf("budget should be omitted when unset, got %d", *b)
	}
}

func TestClientStreamOverHTTP(t *testing.T) {
	var gotPath, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotKey = r.Header.Get("x-goog-api-key")
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"pong"}]},"finishReason":"STOP","index":0}]}`+"\n\n")
	}))
	defer server.Close()

	client, err := New(WithAPIKey("k"), WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	s, err := client.Stream(context.Background(), llm.Request{Messages: []llm.Message{llm.UserText("ping")}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for s.Next() {
	}
	if err := s.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if got := s.Message().Text(); got != "pong" {
		t.Errorf("text = %q", got)
	}
	if want := "/models/gemini-2.5-pro:streamGenerateContent?alt=sse"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotKey != "k" {
		t.Errorf("api key header = %q", gotKey)
	}
}

func TestClientStreamAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"code":400,"message":"bad model","status":"INVALID_ARGUMENT"}}`)
	}))
	defer server.Close()

	client, _ := New(WithAPIKey("k"), WithBaseURL(server.URL), WithMaxRetries(0))
	_, err := client.Stream(context.Background(), llm.Request{Messages: []llm.Message{llm.UserText("x")}})

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.Status != "INVALID_ARGUMENT" || apiErr.Message != "bad model" {
		t.Errorf("error = %+v", apiErr)
	}
	if apiErr.Retryable() {
		t.Error("400 should not be retryable")
	}
}
