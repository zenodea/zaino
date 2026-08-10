package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/gemini"
	"github.com/zenodea/zaino/internal/llm"
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
		Tools: []Tool{{
			Definition: llm.Tool{
				Name:        "echo",
				InputSchema: map[string]any{"type": "object", "additionalProperties": false},
			},
			Run: func(_ context.Context, input json.RawMessage) (string, error) {
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
