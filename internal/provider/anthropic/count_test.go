package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestCountTokens(t *testing.T) {
	var path string
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		io.WriteString(w, `{"input_tokens":198432}`)
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.CountTokens(context.Background(), llm.Request{
		Model:     "claude-opus-5",
		MaxTokens: 4096,
		System:    "be brief",
		Messages:  []llm.Message{llm.UserText("hello")},
		Tools:     []llm.Tool{{Name: "read", Description: "read a file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 198432 {
		t.Errorf("CountTokens = %d, want 198432", got)
	}
	if path != "/v1/messages/count_tokens" {
		t.Errorf("path = %q", path)
	}

	if _, ok := body["stream"]; ok {
		t.Error("count request carried stream")
	}
	if _, ok := body["max_tokens"]; ok {
		t.Error("count request carried max_tokens")
	}
	for _, want := range []string{"model", "messages", "system", "tools"} {
		if _, ok := body[want]; !ok {
			t.Errorf("count request left out %q", want)
		}
	}
}

func TestCountTokensAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`)
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CountTokens(context.Background(), llm.Request{Messages: []llm.Message{llm.UserText("hi")}}); err == nil {
		t.Fatal("CountTokens returned no error on a 400")
	}
}

var _ llm.TokenCounter = (*Client)(nil)
