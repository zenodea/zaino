package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider/anthropic"
	"github.com/zenodea/zaino/internal/tool"
)

type capturedRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []llm.Message `json:"messages"`
	System    []struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		CacheControl *struct {
			Type string `json:"type"`
		} `json:"cache_control"`
	} `json:"system"`
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
	Thinking *struct {
		Type    string `json:"type"`
		Display string `json:"display"`
	} `json:"thinking"`
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
	Stream bool `json:"stream"`
}

type fakeAPI struct {
	turns []string

	mu       sync.Mutex
	requests []capturedRequest
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var req capturedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	n := len(f.requests)
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	if n >= len(f.turns) {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"no more canned turns"}}`,
			http.StatusBadRequest)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	io.WriteString(w, f.turns[n])
}

func (f *fakeAPI) request(i int) capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func newTestAgent(t *testing.T, turns ...string) (*Agent, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{turns: turns}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	client, err := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(server.URL),
		anthropic.WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &Agent{Provider: client, Model: "test-model", MaxTokens: 1024}, api
}

func sseTurn(blocks ...string) string {
	return strings.Join(blocks, "\n\n") + "\n\n"
}

const (
	turnStart = `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`
	turnStop  = `data: {"type":"message_stop"}`
)

func textTurn(text string) string {
	return sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+quote(text)+`}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
		turnStop,
	)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRunExecutesToolsAndContinues(t *testing.T) {
	toolTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"echo","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"v\":\"one\"}"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_b","name":"echo","input":{}}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"v\":\"two\"}"}}`,
		`data: {"type":"content_block_stop","index":1}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":20}}`,
		turnStop,
	)

	ag, api := newTestAgent(t, toolTurn, textTurn("done"))
	ag.Tools = []tool.Tool{&tool.Func{
		Def: llm.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}},
		Do: func(_ context.Context, input json.RawMessage) (string, error) {
			var in struct {
				V string `json:"v"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			return "echo:" + in.V, nil
		},
	}}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4: %+v", len(history), history)
	}
	if history[3].Text() != "done" {
		t.Errorf("final text = %q", history[3].Text())
	}

	results := history[2]
	if results.Role != llm.RoleUser {
		t.Errorf("tool results role = %q, want user", results.Role)
	}
	if len(results.Content) != 2 {
		t.Fatalf("got %d tool results in the message, want 2 — results must not be split "+
			"across messages", len(results.Content))
	}
	for i, want := range []struct{ id, content string }{
		{"toolu_a", "echo:one"},
		{"toolu_b", "echo:two"},
	} {
		block, ok := results.Content[i].(llm.ToolResultBlock)
		if !ok {
			t.Fatalf("result %d: got %T", i, results.Content[i])
		}
		if block.ToolUseID != want.id || block.Content != want.content || block.IsError {
			t.Errorf("result %d = %+v, want id=%s content=%s", i, block, want.id, want.content)
		}
	}

	if got := len(api.request(1).Messages); got != 3 {
		t.Errorf("second request carried %d messages, want 3", got)
	}
}

func TestRunToolErrorBecomesResult(t *testing.T) {
	toolTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"boom","input":{}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)

	ag, _ := newTestAgent(t, toolTurn, textTurn("recovered"))
	ag.Tools = []tool.Tool{&tool.Func{
		Def: llm.Tool{Name: "boom", InputSchema: map[string]any{"type": "object"}},
		Do: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("disk on fire")
		},
	}}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	block := history[2].Content[0].(llm.ToolResultBlock)
	if !block.IsError {
		t.Error("is_error not set on a failed tool")
	}
	if !strings.Contains(block.Content, "disk on fire") {
		t.Errorf("result = %q, want the tool's error text", block.Content)
	}
}

func TestRunUnknownToolBecomesResult(t *testing.T) {
	toolTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"ghost","input":{}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)

	ag, _ := newTestAgent(t, toolTurn, textTurn("ok"))
	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	block := history[2].Content[0].(llm.ToolResultBlock)
	if !block.IsError || !strings.Contains(block.Content, "ghost") {
		t.Errorf("result = %+v", block)
	}
}

func TestRunResumesPauseTurn(t *testing.T) {
	pauseTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"searching"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"pause_turn","stop_sequence":null},"usage":{"output_tokens":4}}`,
		turnStop,
	)

	ag, api := newTestAgent(t, pauseTurn, textTurn("finished"))
	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("history has %d messages, want 3: %+v", len(history), history)
	}
	second := api.request(1).Messages
	if second[len(second)-1].Role != llm.RoleAssistant {
		t.Errorf("resume request should end on the paused assistant turn, ends on %q",
			second[len(second)-1].Role)
	}
}

func TestRunRefusal(t *testing.T) {
	refusalTurn := sseTurn(
		turnStart,
		`data: {"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null,"stop_details":{"type":"refusal","category":"cyber","explanation":"declined"}},"usage":{"output_tokens":0}}`,
		turnStop,
	)

	ag, _ := newTestAgent(t, refusalTurn)
	_, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})

	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("got %v (%T), want *RefusalError", err, err)
	}
	if refusal.Details == nil || refusal.Details.Category != "cyber" {
		t.Errorf("details = %+v", refusal.Details)
	}
}

func TestRunMaxTurns(t *testing.T) {
	toolTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"spin","input":{}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)

	ag, _ := newTestAgent(t, toolTurn, toolTurn, toolTurn)
	ag.MaxTurns = 2
	ag.Tools = []tool.Tool{&tool.Func{
		Def: llm.Tool{Name: "spin", InputSchema: map[string]any{"type": "object"}},
		Do:  func(context.Context, json.RawMessage) (string, error) { return "again", nil },
	}}

	_, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if !errors.Is(err, ErrMaxTurns) {
		t.Fatalf("got %v, want ErrMaxTurns", err)
	}
}

func TestSystemPromptIsCachedAndStable(t *testing.T) {
	ag, api := newTestAgent(t, textTurn("hi"))
	ag.System = "You are Zaino."
	ag.Effort = llm.EffortXHigh

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")}); err != nil {
		t.Fatal(err)
	}

	req := api.request(0)
	if len(req.System) != 1 {
		t.Fatalf("got %d system blocks, want 1", len(req.System))
	}
	if req.System[0].Text != "You are Zaino." {
		t.Errorf("system text = %q", req.System[0].Text)
	}
	if req.System[0].CacheControl == nil || req.System[0].CacheControl.Type != "ephemeral" {
		t.Errorf("system block missing ephemeral cache_control: %+v", req.System[0])
	}
	if req.OutputConfig == nil || req.OutputConfig.Effort != llm.EffortXHigh {
		t.Errorf("output_config = %+v", req.OutputConfig)
	}
	if !req.Stream {
		t.Error("Stream should be true on the wire")
	}
}
