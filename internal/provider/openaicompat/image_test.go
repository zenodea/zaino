package openaicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestImageTravelsAsADataURI(t *testing.T) {
	req, err := buildRequest(llm.Request{Messages: []llm.Message{{
		Role: llm.RoleUser,
		Content: llm.Content{
			llm.TextBlock{Text: "why is this wrong"},
			llm.ImageBlock{MediaType: "image/png", Data: []byte("bytes")},
		},
	}}}, Config{}, "gpt-5")
	if err != nil {
		t.Fatal(err)
	}

	parts, ok := req.Messages[0].Content.([]contentPart)
	if !ok {
		t.Fatalf("content = %T, want parts", req.Messages[0].Content)
	}
	if len(parts) != 2 || parts[0].Type != "text" || parts[1].Type != "image_url" {
		t.Fatalf("parts = %+v", parts)
	}
	if want := "data:image/png;base64,Ynl0ZXM="; parts[1].ImageURL.URL != want {
		t.Errorf("url = %q, want %q", parts[1].ImageURL.URL, want)
	}
}

// Every compatible host has always understood a plain string, so a message
// with no picture in it must not start arriving as an array.
func TestTextOnlyMessagesStayStrings(t *testing.T) {
	req, err := buildRequest(llm.Request{
		System:   "be terse",
		Messages: []llm.Message{llm.UserText("hello")},
	}, Config{}, "gpt-5")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"content":"hello"`) {
		t.Errorf("a plain message did not go out as a string: %s", raw)
	}
	for _, message := range req.Messages {
		if _, ok := message.Content.(string); !ok {
			t.Errorf("content = %T, want a string", message.Content)
		}
	}
}
