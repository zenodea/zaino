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

func TestCountTokens(t *testing.T) {
	var path string
	var body struct {
		Request struct {
			Model             string `json:"model"`
			Contents          []any  `json:"contents"`
			SystemInstruction *any   `json:"systemInstruction"`
			Tools             []any  `json:"tools"`
		} `json:"generateContentRequest"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		io.WriteString(w, `{"totalTokens":198432}`)
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("test-key"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.CountTokens(context.Background(), llm.Request{
		Model:    "gemini-2.5-pro",
		System:   "be brief",
		Messages: []llm.Message{llm.UserText("hello")},
		Tools:    []llm.Tool{{Name: "read", Description: "read a file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 198432 {
		t.Errorf("CountTokens = %d, want 198432", got)
	}
	if !strings.HasSuffix(path, "/models/gemini-2.5-pro:countTokens") {
		t.Errorf("path = %q", path)
	}

	if body.Request.Model != "models/gemini-2.5-pro" {
		t.Errorf("model = %q, want models/gemini-2.5-pro", body.Request.Model)
	}
	if len(body.Request.Contents) != 1 {
		t.Errorf("contents = %d, want 1", len(body.Request.Contents))
	}
	if body.Request.SystemInstruction == nil {
		t.Error("count request left the system instruction out; it is part of the window")
	}
	if len(body.Request.Tools) != 1 {
		t.Error("count request left the tool declarations out; they are part of the window")
	}
}

// The model belongs in the body only where the endpoint asks for it there.
func TestStreamRequestCarriesNoModelField(t *testing.T) {
	req, err := buildRequest(llm.Request{Messages: []llm.Message{llm.UserText("hi")}}, 8192)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"model"`) {
		t.Errorf("stream request body carried a model field: %s", raw)
	}
}

var _ llm.TokenCounter = (*Client)(nil)
