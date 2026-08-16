package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
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

func TestStreamAssemblesText(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","model":"test-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":", world"}}]}`,
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":4}}`,
		doneMarker,
	))
	defer s.Close()

	resp := drain(t, s)
	if resp.ID != "chatcmpl-1" || resp.Model != "test-1" {
		t.Errorf("header not carried: %+v", resp)
	}
	if resp.Role != llm.RoleAssistant {
		t.Errorf("role = %q", resp.Role)
	}
	if got := resp.Text(); got != "Hello, world" {
		t.Errorf("text = %q", got)
	}
	if resp.StopReason != llm.StopEndTurn {
		t.Errorf("stop reason = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestStreamAssemblesAToolCallFromFragments(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"on it"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"/etc/hosts\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		doneMarker,
	))
	defer s.Close()

	resp := drain(t, s)
	if resp.StopReason != llm.StopToolUse {
		t.Errorf("stop reason = %q, want tool_use", resp.StopReason)
	}
	if got := resp.Text(); got != "on it" {
		t.Errorf("text = %q", got)
	}

	uses := resp.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("got %d tool uses, want 1", len(uses))
	}
	if uses[0].ID != "call_abc" || uses[0].Name != "read" {
		t.Errorf("call = %+v", uses[0])
	}
	if got := string(uses[0].Input); got != `{"path":"/etc/hosts"}` {
		t.Errorf("input = %s", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal(uses[0].Input, &decoded); err != nil {
		t.Fatalf("input is not valid json: %v", err)
	}
}

func TestStreamHandlesSeveralToolCalls(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":\"/a\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"ls","arguments":"{\"dir\":\"/b\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		doneMarker,
	))
	defer s.Close()

	uses := drain(t, s).ToolUses()
	if len(uses) != 2 {
		t.Fatalf("got %d tool uses, want 2", len(uses))
	}
	if uses[0].ID != "call_1" || uses[0].Name != "read" {
		t.Errorf("call 0 = %+v", uses[0])
	}
	if uses[1].ID != "call_2" || uses[1].Name != "ls" {
		t.Errorf("call 1 = %+v", uses[1])
	}
	if got := string(uses[1].Input); got != `{"dir":"/b"}` {
		t.Errorf("call 1 input = %s", got)
	}
}

// Nothing in the format forbids the host from interleaving two calls.
func TestStreamHandlesInterleavedToolCalls(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"ls","arguments":"{\"dir\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/a\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"/b\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		doneMarker,
	))
	defer s.Close()

	uses := drain(t, s).ToolUses()
	if len(uses) != 2 {
		t.Fatalf("got %d tool uses, want 2", len(uses))
	}
	if got := string(uses[0].Input); got != `{"path":"/a"}` {
		t.Errorf("call 0 input = %s", got)
	}
	if got := string(uses[1].Input); got != `{"dir":"/b"}` {
		t.Errorf("call 1 input = %s", got)
	}
}

// Grok streams reasoning here; OpenAI never sends the field.
func TestStreamAssemblesReasoning(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"let me "}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"count"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"four"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		doneMarker,
	))
	defer s.Close()

	resp := drain(t, s)
	if len(resp.Content) != 2 {
		t.Fatalf("got %d blocks, want thinking then text: %+v", len(resp.Content), resp.Content)
	}
	think, ok := resp.Content[0].(llm.ThinkingBlock)
	if !ok || think.Thinking != "let me count" {
		t.Errorf("block 0 = %#v", resp.Content[0])
	}
	if got := resp.Text(); got != "four" {
		t.Errorf("text = %q — reasoning must not leak into it", got)
	}
}

func TestStreamReportsARefusal(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","refusal":"I cannot "}}]}`,
		`{"choices":[{"index":0,"delta":{"refusal":"help with that"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		doneMarker,
	))
	defer s.Close()

	resp := drain(t, s)
	if resp.StopReason != llm.StopRefusal {
		t.Errorf("stop reason = %q, want refusal", resp.StopReason)
	}
	if resp.StopDetails == nil || resp.StopDetails.Explanation != "I cannot help with that" {
		t.Errorf("stop details = %+v", resp.StopDetails)
	}
}

func TestStreamReportsAContentFilter(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`,
		doneMarker,
	))
	defer s.Close()

	resp := drain(t, s)
	if resp.StopReason != llm.StopRefusal {
		t.Errorf("stop reason = %q, want refusal", resp.StopReason)
	}
	if resp.StopDetails == nil || resp.StopDetails.Category != "content_filter" {
		t.Errorf("stop details = %+v", resp.StopDetails)
	}
}

func TestStreamReportsTruncation(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"as far as"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`,
		doneMarker,
	))
	defer s.Close()

	if got := drain(t, s).StopReason; got != llm.StopMaxTokens {
		t.Errorf("stop reason = %q, want max_tokens", got)
	}
}

func TestStreamEndsEveryBlockItOpened(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"hmm"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"here"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		doneMarker,
	))
	defer s.Close()

	opened := map[int]bool{}
	closed := map[int]bool{}
	var stopped bool
	for s.Next() {
		switch ev := s.Event().(type) {
		case llm.ContentBlockStartEvent:
			if opened[ev.Index] {
				t.Errorf("block %d opened twice", ev.Index)
			}
			opened[ev.Index] = true
		case llm.ContentBlockStopEvent:
			if !opened[ev.Index] {
				t.Errorf("block %d closed before it opened", ev.Index)
			}
			if closed[ev.Index] {
				t.Errorf("block %d closed twice", ev.Index)
			}
			closed[ev.Index] = true
		case llm.MessageStopEvent:
			stopped = true
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 3 {
		t.Errorf("opened %d blocks, want 3", len(opened))
	}
	for index := range opened {
		if !closed[index] {
			t.Errorf("block %d was never closed", index)
		}
	}
	if !stopped {
		t.Error("the stream never said it stopped")
	}
}

func TestStreamIgnoresOtherChoices(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"first"}}]}`,
		`{"choices":[{"index":1,"delta":{"content":" second"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		doneMarker,
	))
	defer s.Close()

	if got := drain(t, s).Text(); got != "first" {
		t.Errorf("text = %q, want just the first choice", got)
	}
}

func TestStreamSurvivesAMissingDoneMarker(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	))
	defer s.Close()

	if got := drain(t, s).Text(); got != "hi" {
		t.Errorf("text = %q", got)
	}
}

// A connection that drops mid-turn must not look like a finished answer.
func TestStreamRefusesToEndWithoutAFinishReason(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"half an ans"}}]}`,
	))
	defer s.Close()

	for s.Next() {
	}
	if err := s.Err(); err == nil {
		t.Fatal("got nil, want a truncation error")
	} else if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want an unexpected EOF", err)
	}
}

func TestStreamRejectsAnEmptyStream(t *testing.T) {
	s := newStream("test", sseBody(doneMarker))
	defer s.Close()

	for s.Next() {
	}
	if s.Err() == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestStreamRejectsMalformedChunks(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		`{not json`,
	))
	defer s.Close()

	for s.Next() {
	}
	if err := s.Err(); err == nil {
		t.Fatal("got nil, want an error")
	} else if !strings.Contains(err.Error(), "malformed") {
		t.Errorf("got %v", err)
	}
}

func TestStreamSurfacesAnInBandError(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		`{"error":{"type":"server_error","message":"upstream fell over"}}`,
	))
	defer s.Close()

	for s.Next() {
	}
	var api *APIError
	if !errors.As(s.Err(), &api) {
		t.Fatalf("got %T (%v), want *APIError", s.Err(), s.Err())
	}
	if api.Message != "upstream fell over" {
		t.Errorf("message = %q", api.Message)
	}
}

func TestStreamInventsAnIDWhenTheHostOmitsOne(t *testing.T) {
	s := newStream("test", sseBody(
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		doneMarker,
	))
	defer s.Close()

	uses := drain(t, s).ToolUses()
	if len(uses) != 1 {
		t.Fatalf("got %d tool uses, want 1", len(uses))
	}
	if uses[0].ID == "" {
		t.Error("the tool call has no id, so its result can never be matched")
	}
}

func TestClientStreamsOverHTTP(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("authorization")
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"chatcmpl-1\",\"model\":\"test-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"pong\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c, err := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	st, err := c.Stream(context.Background(), llm.Request{Messages: []llm.Message{llm.UserText("ping")}})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for st.Next() {
	}
	if err := st.Err(); err != nil {
		t.Fatal(err)
	}
	if got := st.Message().Text(); got != "pong" {
		t.Errorf("text = %q", got)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if !gotBody.Stream {
		t.Error("the request did not ask to stream")
	}
}

func TestClientReportsAnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req_9")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"model_not_found","message":"no such model"}}`)
	}))
	defer srv.Close()

	c, err := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Stream(context.Background(), llm.Request{})
	var api *APIError
	if !errors.As(err, &api) {
		t.Fatalf("got %T (%v), want *APIError", err, err)
	}
	if api.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d", api.StatusCode)
	}
	if api.Code != "model_not_found" || api.RequestID != "req_9" {
		t.Errorf("error = %+v", api)
	}
	if !strings.Contains(api.Error(), "test:") {
		t.Errorf("the error does not name the provider: %v", api)
	}
}
