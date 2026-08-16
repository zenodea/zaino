package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider/gemini"
	"github.com/zenodea/zaino/internal/provider/openaicompat"
	"github.com/zenodea/zaino/internal/tool"
)

func TestRunAgainstGeminiProvider(t *testing.T) {
	turns := []string{
		`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"echo","args":{"v":"one"}}},{"functionCall":{"name":"echo","args":{"v":"two"}}}]},"finishReason":"STOP","index":0}]}` + "\n\n",
		`data: {"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP","index":0}]}` + "\n\n",
	}

	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := len(bodies)
		bodies = append(bodies, string(body))
		if n >= len(turns) {
			http.Error(w, `{"error":{"message":"no more canned turns"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, turns[n])
	}))
	defer server.Close()

	provider, err := gemini.New(
		gemini.WithAPIKey("test-key"),
		gemini.WithBaseURL(server.URL),
		gemini.WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}

	ag := &Agent{
		Provider:  provider,
		MaxTokens: 1024,
		Tools: []tool.Tool{&tool.Func{
			Def: llm.Tool{
				Name:        "echo",
				InputSchema: map[string]any{"type": "object", "additionalProperties": false},
			},
			Do: func(_ context.Context, input json.RawMessage) (string, error) {
				var in struct {
					V string `json:"v"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return "", err
				}
				return "echo:" + in.V, nil
			},
		}},
	}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4: %+v", len(history), history)
	}
	if got := history[3].Text(); got != "done" {
		t.Errorf("final text = %q", got)
	}
	if len(history[2].Content) != 2 {
		t.Fatalf("got %d tool results, want 2", len(history[2].Content))
	}

	second := bodies[1]
	for _, want := range []string{`"functionResponse"`, `"name":"echo"`, `"output":"echo:one"`, `"output":"echo:two"`} {
		if !strings.Contains(second, want) {
			t.Errorf("second request missing %s:\n%s", want, second)
		}
	}

	if strings.Contains(second, "zn-") {
		t.Errorf("synthetic tool-call id leaked to the wire:\n%s", second)
	}

	if strings.Contains(bodies[0], "additionalProperties") {
		t.Errorf("additionalProperties leaked into the tool declaration:\n%s", bodies[0])
	}
}

func TestRunAgainstAnOpenAICompatibleProvider(t *testing.T) {
	turns := []string{
		`data: {"id":"chatcmpl-1","model":"test-1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"v\":\"one\"}"}}]}}]}` + "\n\n" +
			`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"echo","arguments":"{\"v\":\"two\"}"}}]}}]}` + "\n\n" +
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
		`data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"role":"assistant","content":"done"}}]}` + "\n\n" +
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
	}

	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := len(bodies)
		bodies = append(bodies, string(body))
		if n >= len(turns) {
			http.Error(w, `{"error":{"message":"no more canned turns"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, turns[n])
	}))
	defer server.Close()

	provider, err := openaicompat.New(
		openaicompat.Config{Name: "test", BaseURL: server.URL, DefaultModel: "test-1"},
		openaicompat.WithAPIKey("test-key"),
		openaicompat.WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}

	ag := &Agent{
		Provider:  provider,
		MaxTokens: 1024,
		Tools: []tool.Tool{&tool.Func{
			Def: llm.Tool{
				Name:        "echo",
				InputSchema: map[string]any{"type": "object", "additionalProperties": false},
			},
			Do: func(_ context.Context, input json.RawMessage) (string, error) {
				var in struct {
					V string `json:"v"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return "", err
				}
				return "echo:" + in.V, nil
			},
		}},
	}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("history has %d messages, want 4: %+v", len(history), history)
	}
	if got := history[3].Text(); got != "done" {
		t.Errorf("final text = %q", got)
	}
	if len(history[2].Content) != 2 {
		t.Errorf("both tool results should come back in one message: %+v", history[2])
	}

	// The second request has to carry each result as its own tool message,
	// keyed by the id the first turn handed out.
	if len(bodies) != 2 {
		t.Fatalf("made %d requests, want 2", len(bodies))
	}
	for _, want := range []string{`"call_1"`, `"call_2"`, `"echo:one"`, `"echo:two"`, `"role":"tool"`} {
		if !strings.Contains(bodies[1], want) {
			t.Errorf("the follow-up request is missing %s:\n%s", want, bodies[1])
		}
	}
}
