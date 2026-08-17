package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func toolUse(signature string) llm.Message {
	return llm.Message{
		Role: llm.RoleAssistant,
		Content: llm.Content{llm.ToolUseBlock{
			ID:        "toolu_a",
			Name:      "ls",
			Input:     json.RawMessage(`{"path":"."}`),
			Signature: signature,
		}},
	}
}

func TestToolSignatureNeverReachesTheWire(t *testing.T) {
	req := buildRequest(llm.Request{Messages: []llm.Message{toolUse("gemini-signature")}}, "claude-opus-5")

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "signature") {
		t.Errorf("request carries a signature it should have stripped: %s", raw)
	}
}

func TestStrippingLeavesTheOriginalAlone(t *testing.T) {
	messages := []llm.Message{toolUse("gemini-signature")}
	buildRequest(llm.Request{Messages: messages}, "claude-opus-5")

	use := messages[0].Content[0].(llm.ToolUseBlock)
	if use.Signature != "gemini-signature" {
		t.Errorf("Signature = %q, want the caller's copy untouched", use.Signature)
	}
}

func TestHistoryWithoutSignaturesIsPassedThrough(t *testing.T) {
	messages := []llm.Message{toolUse("")}
	req := buildRequest(llm.Request{Messages: messages}, "claude-opus-5")

	if &req.Messages.messages[0] != &messages[0] {
		t.Error("a history with nothing to strip was copied anyway")
	}
}
