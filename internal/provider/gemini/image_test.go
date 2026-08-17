package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestImageTravelsAsInlineData(t *testing.T) {
	req, err := buildRequest(llm.Request{Messages: []llm.Message{{
		Role: llm.RoleUser,
		Content: llm.Content{
			llm.TextBlock{Text: "why is this wrong"},
			llm.ImageBlock{MediaType: "image/png", Data: []byte("bytes")},
		},
	}}}, 4096)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"inlineData"`) || !strings.Contains(string(raw), `"image/png"`) {
		t.Errorf("request has no picture in it: %s", raw)
	}

	parts := req.Contents[0].Parts
	if len(parts) != 2 || parts[1].InlineData == nil {
		t.Fatalf("parts = %+v", parts)
	}
	if string(parts[1].InlineData.Data) != "bytes" {
		t.Errorf("data = %q", parts[1].InlineData.Data)
	}
}
