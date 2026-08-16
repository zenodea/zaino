package openaicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

var testConfig = Config{
	Name:            "test",
	BaseURL:         "https://api.example/v1",
	DefaultModel:    "test-1",
	KnownModels:     []string{"test-1", "test-2"},
	APIKeyEnv:       []string{"TEST_API_KEY"},
	BaseURLEnv:      "TEST_BASE_URL",
	ReasoningEffort: true,
}

func TestBuildRequestBasics(t *testing.T) {
	got, err := buildRequest(llm.Request{
		System:    "be terse",
		MaxTokens: 4096,
		Messages:  []llm.Message{llm.UserText("hello")},
	}, testConfig, "test-1")
	if err != nil {
		t.Fatal(err)
	}

	if got.Model != "test-1" {
		t.Errorf("model = %q", got.Model)
	}
	if !got.Stream || got.StreamOptions == nil || !got.StreamOptions.IncludeUsage {
		t.Error("usage must be requested alongside the stream")
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	if got.Messages[0].Role != "system" || got.Messages[0].Content != "be terse" {
		t.Errorf("system message = %+v", got.Messages[0])
	}
	if got.Messages[1].Role != "user" || got.Messages[1].Content != "hello" {
		t.Errorf("user message = %+v", got.Messages[1])
	}
}

func TestBuildRequestPrefersAnExplicitModel(t *testing.T) {
	got, _ := buildRequest(llm.Request{Model: "test-2"}, testConfig, "test-1")
	if got.Model != "test-2" {
		t.Errorf("model = %q, want test-2", got.Model)
	}
}

// Newer OpenAI models reject max_tokens; older compatible hosts reject the
// replacement, so the field is chosen per vendor.
func TestMaxTokensFieldFollowsTheVendor(t *testing.T) {
	modern, _ := buildRequest(llm.Request{MaxTokens: 100}, testConfig, "test-1")
	if modern.MaxCompletion == nil || *modern.MaxCompletion != 100 {
		t.Errorf("max_completion_tokens = %v", modern.MaxCompletion)
	}
	if modern.MaxTokens != nil {
		t.Error("max_tokens was also set")
	}

	legacy := testConfig
	legacy.LegacyMaxTokens = true
	old, _ := buildRequest(llm.Request{MaxTokens: 100}, legacy, "test-1")
	if old.MaxTokens == nil || *old.MaxTokens != 100 {
		t.Errorf("max_tokens = %v", old.MaxTokens)
	}
	if old.MaxCompletion != nil {
		t.Error("max_completion_tokens was also set")
	}
}

func TestUnsetMaxTokensIsOmitted(t *testing.T) {
	got, _ := buildRequest(llm.Request{}, testConfig, "test-1")
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "max_") {
		t.Errorf("a token limit leaked into the request: %s", body)
	}
}

func TestDeveloperRoleReplacesSystem(t *testing.T) {
	cfg := testConfig
	cfg.DeveloperRole = true
	got, _ := buildRequest(llm.Request{System: "be terse"}, cfg, "test-1")
	if got.Messages[0].Role != "developer" {
		t.Errorf("role = %q, want developer", got.Messages[0].Role)
	}
}

func TestReasoningEffortMapping(t *testing.T) {
	cases := map[string]string{
		llm.EffortLow:    "low",
		llm.EffortMedium: "medium",
		llm.EffortHigh:   "high",
		llm.EffortXHigh:  "high",
		llm.EffortMax:    "high",
		"":               "",
	}
	for effort, want := range cases {
		got, _ := buildRequest(llm.Request{Effort: effort}, testConfig, "test-1")
		if got.ReasoningEffort != want {
			t.Errorf("effort %q became %q, want %q", effort, got.ReasoningEffort, want)
		}
	}
}

func TestReasoningEffortIsWithheldFromHostsThatRejectIt(t *testing.T) {
	cfg := testConfig
	cfg.ReasoningEffort = false
	got, _ := buildRequest(llm.Request{Effort: llm.EffortHigh}, cfg, "test-1")
	if got.ReasoningEffort != "" {
		t.Errorf("reasoning_effort = %q, want it omitted", got.ReasoningEffort)
	}
}

func TestToolsAreDeclared(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
	got, _ := buildRequest(llm.Request{
		Tools: []llm.Tool{{Name: "read", Description: "read a file", InputSchema: schema}},
	}, testConfig, "test-1")

	if len(got.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(got.Tools))
	}
	tool := got.Tools[0]
	if tool.Type != "function" {
		t.Errorf("type = %q", tool.Type)
	}
	if tool.Function.Name != "read" || tool.Function.Description != "read a file" {
		t.Errorf("function = %+v", tool.Function)
	}
	if tool.Function.Parameters["type"] != "object" {
		t.Errorf("parameters = %v", tool.Function.Parameters)
	}
}

func TestAssistantToolCallsRoundTrip(t *testing.T) {
	got, err := buildRequest(llm.Request{Messages: []llm.Message{
		llm.UserText("read it"),
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.TextBlock{Text: "on it"},
			llm.ToolUseBlock{ID: "call_1", Name: "read", Input: json.RawMessage(`{"path":"/x"}`)},
		}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "call_1", Content: "file contents"},
		}},
	}}, testConfig, "test-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Messages) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(got.Messages), got.Messages)
	}
	assistant := got.Messages[1]
	if assistant.Role != "assistant" || assistant.Content != "on it" {
		t.Errorf("assistant = %+v", assistant)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(assistant.ToolCalls))
	}
	call := assistant.ToolCalls[0]
	if call.ID != "call_1" || call.Type != "function" || call.Function.Name != "read" {
		t.Errorf("call = %+v", call)
	}
	if call.Function.Arguments != `{"path":"/x"}` {
		t.Errorf("arguments = %q", call.Function.Arguments)
	}

	result := got.Messages[2]
	if result.Role != roleTool || result.ToolCallID != "call_1" || result.Content != "file contents" {
		t.Errorf("tool result = %+v", result)
	}
}

// The wire format wants every tool result as its own message, ahead of any
// text the same turn carried.
func TestSeveralToolResultsBecomeSeveralMessages(t *testing.T) {
	got, err := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ToolUseBlock{ID: "call_1", Name: "read"},
			llm.ToolUseBlock{ID: "call_2", Name: "ls"},
		}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "call_1", Content: "one"},
			llm.ToolResultBlock{ToolUseID: "call_2", Content: "two"},
			llm.TextBlock{Text: "and now this"},
		}},
	}}, testConfig, "test-1")
	if err != nil {
		t.Fatal(err)
	}

	roles := make([]string, len(got.Messages))
	for i, m := range got.Messages {
		roles[i] = m.Role
	}
	want := []string{"assistant", roleTool, roleTool, "user"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("roles = %v, want %v", roles, want)
	}
	if got.Messages[3].Content != "and now this" {
		t.Errorf("trailing text = %q", got.Messages[3].Content)
	}
}

func TestBuildRequestRejectsAnOrphanToolResult(t *testing.T) {
	_, err := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "call_missing", Content: "x"},
		}},
	}}, testConfig, "test-1")
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "call_missing") {
		t.Errorf("the error does not name the call: %v", err)
	}
}

func TestAToolCallWithNoArgumentsSendsAnObject(t *testing.T) {
	got, _ := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{llm.ToolUseBlock{ID: "call_1", Name: "now"}}},
	}}, testConfig, "test-1")
	if arg := got.Messages[0].ToolCalls[0].Function.Arguments; arg != "{}" {
		t.Errorf("arguments = %q, want {}", arg)
	}
}

func TestAnEmptyToolResultStillSaysSomething(t *testing.T) {
	got, _ := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{llm.ToolUseBlock{ID: "call_1", Name: "ls"}}},
		{Role: llm.RoleUser, Content: llm.Content{llm.ToolResultBlock{ToolUseID: "call_1"}}},
	}}, testConfig, "test-1")

	if got.Messages[1].Content == "" {
		t.Error("an empty tool result would be dropped by the server")
	}
}

// Chat Completions has nowhere to put reasoning, and replaying it as
// assistant text would have the model answer its own thinking.
func TestThinkingIsNotReplayed(t *testing.T) {
	got, _ := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ThinkingBlock{Thinking: "hmm", Signature: "sig"},
			llm.TextBlock{Text: "the answer"},
		}},
	}}, testConfig, "test-1")

	if len(got.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(got.Messages))
	}
	if got.Messages[0].Content != "the answer" {
		t.Errorf("content = %q", got.Messages[0].Content)
	}
	body, _ := json.Marshal(got)
	if strings.Contains(string(body), "hmm") {
		t.Errorf("reasoning reached the wire: %s", body)
	}
}

func TestAMessageWithNothingToSayIsDropped(t *testing.T) {
	got, _ := buildRequest(llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{llm.ThinkingBlock{Thinking: "hmm"}}},
	}}, testConfig, "test-1")
	if len(got.Messages) != 0 {
		t.Errorf("got %+v, want nothing", got.Messages)
	}
}

func TestUsageMapping(t *testing.T) {
	u := usage{PromptTokens: 100, CompletionTokens: 20}
	u.PromptTokensDetails = &struct {
		CachedTokens int `json:"cached_tokens"`
	}{CachedTokens: 40}
	u.CompletionTokensDetails = &struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	}{ReasoningTokens: 8}

	got := u.toLLM()
	// prompt_tokens is inclusive of the cached part; the rest of zaino counts
	// them separately.
	want := llm.Usage{InputTokens: 60, OutputTokens: 20, ThinkingTokens: 8, CacheReadTokens: 40}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestUsageWithoutDetails(t *testing.T) {
	got := usage{PromptTokens: 10, CompletionTokens: 3}.toLLM()
	if want := (llm.Usage{InputTokens: 10, OutputTokens: 3}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestStopReasonMapping(t *testing.T) {
	cases := []struct {
		finish  string
		sawTool bool
		want    llm.StopReason
	}{
		{"stop", false, llm.StopEndTurn},
		{"length", false, llm.StopMaxTokens},
		{"tool_calls", true, llm.StopToolUse},
		{"function_call", true, llm.StopToolUse},
		{"content_filter", false, llm.StopRefusal},
		{"", true, llm.StopToolUse},
		{"", false, llm.StopEndTurn},
	}
	for _, c := range cases {
		got, _ := stopReason(c.finish, c.sawTool)
		if got != c.want {
			t.Errorf("stopReason(%q, %v) = %q, want %q", c.finish, c.sawTool, got, c.want)
		}
	}
	if _, details := stopReason("content_filter", false); details == nil {
		t.Error("a filtered response should explain itself")
	}
}
