package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

// A breakpoint is a marker on a block, and the wire is the only place to see one.
func marked(t *testing.T, req wireRequest) []string {
	t.Helper()
	raw, err := json.Marshal(req.Messages)
	if err != nil {
		t.Fatal(err)
	}

	var messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type         string          `json:"type"`
			Text         string          `json:"text"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}

	var out []string
	for _, message := range messages {
		for _, block := range message.Content {
			if len(block.CacheControl) > 0 {
				out = append(out, block.Text)
			}
		}
	}
	return out
}

func conversation() []llm.Message {
	return []llm.Message{
		llm.UserText("first"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "answer one"}}},
		llm.UserText("second"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "answer two"}}},
		llm.UserText("third"),
	}
}

// The turn just finished, so the next one reads it back; and the end of the
// turn before it, which is the prefix the last request wrote.
func TestConversationIsCachedAtBothEnds(t *testing.T) {
	req := buildRequest(llm.Request{Messages: conversation()}, "claude-opus-5")

	got := marked(t, req)
	if len(got) != 2 || got[0] != "second" || got[1] != "third" {
		t.Errorf("breakpoints on %v, want the last two user turns", got)
	}
}

func TestTheFirstTurnIsStillWorthKeeping(t *testing.T) {
	req := buildRequest(llm.Request{Messages: []llm.Message{llm.UserText("first")}}, "claude-opus-5")

	if got := marked(t, req); len(got) != 1 || got[0] != "first" {
		t.Errorf("breakpoints on %v, want the one message there is", got)
	}
}

func TestTheMarkerGoesOnTheLastBlock(t *testing.T) {
	messages := []llm.Message{{Role: llm.RoleUser, Content: llm.Content{
		llm.TextBlock{Text: "one"}, llm.TextBlock{Text: "two"},
	}}}

	if got := marked(t, buildRequest(llm.Request{Messages: messages}, "claude-opus-5")); len(got) != 1 || got[0] != "two" {
		t.Errorf("breakpoints on %v, want the last block alone", got)
	}
}

func TestSystemAndConversationAreBothCached(t *testing.T) {
	req := buildRequest(llm.Request{System: "be terse", Messages: conversation()}, "claude-opus-5")

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	// One on the system block, two in the conversation, and Anthropic allows
	// four — going over is a 400, so the count is part of the contract.
	if n := strings.Count(string(raw), "cache_control"); n != 3 {
		t.Errorf("%d breakpoints, want 3:\n%s", n, raw)
	}
}

func TestMarkingLeavesTheBlocksAsTheyWere(t *testing.T) {
	messages := conversation()
	if _, err := json.Marshal(buildRequest(llm.Request{Messages: messages}, "claude-opus-5")); err != nil {
		t.Fatal(err)
	}

	block := messages[4].Content[0].(llm.TextBlock)
	if block.Text != "third" {
		t.Errorf("the caller's history came back as %+v", block)
	}
	raw, err := json.Marshal(messages[4])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "cache_control") {
		t.Errorf("the marker stuck to the caller's copy: %s", raw)
	}
}

// The count endpoint has to see what the messages endpoint sees, or the
// number it gives back is for a different request.
func TestCountRequestCarriesTheSameBreakpoints(t *testing.T) {
	req := llm.Request{System: "be terse", Messages: conversation()}

	full, err := json.Marshal(buildRequest(req, "claude-opus-5").Messages)
	if err != nil {
		t.Fatal(err)
	}
	counted, err := json.Marshal(buildCountRequest(req, "claude-opus-5").Messages)
	if err != nil {
		t.Fatal(err)
	}
	if string(full) != string(counted) {
		t.Errorf("count sees\n%s\nand the turn sees\n%s", counted, full)
	}
}
