package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func long(role llm.Role, chars int) llm.Message {
	return llm.Message{Role: role, Content: llm.Content{llm.TextBlock{Text: strings.Repeat("x", chars)}}}
}

func conversation(pairs, chars int) []llm.Message {
	var out []llm.Message
	for range pairs {
		out = append(out, long(llm.RoleUser, chars), long(llm.RoleAssistant, chars))
	}
	return out
}

func TestCompactionTriggersOnlyWhenFull(t *testing.T) {
	tests := []struct {
		name string
		used int
		want bool
	}{
		{"room to spare", 1000, false},
		{"near the edge", 40_000 - 16_384 - 1, false},
		{"over the budget", 40_000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{Compaction: &Compaction{Window: 40_000}}
			a.used = tt.used
			if got := a.shouldCompact(conversation(4, 100)); got != tt.want {
				t.Errorf("shouldCompact = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompactionOffByDefault(t *testing.T) {
	a := &Agent{}
	a.used = 10_000_000
	if a.shouldCompact(conversation(4, 100)) {
		t.Error("compacted with no Compaction configured")
	}
}

func TestCutKeepsTheRecentWindow(t *testing.T) {
	history := conversation(10, 4000) // ~1000 tokens each

	cut := cutPoint(history, 3000)
	if cut == 0 {
		t.Fatal("nothing was cut")
	}
	kept := estimateTokens(history[cut:])
	if kept < 3000 {
		t.Errorf("kept ~%d tokens, want at least the 3000 asked for", kept)
	}
	if kept > 6000 {
		t.Errorf("kept ~%d tokens, far more than the 3000 asked for", kept)
	}
}

// A tool result whose call is on the other side of the cut is rejected by the
// provider, so the cut has to move off it.
func TestCutNeverStrandsAToolResult(t *testing.T) {
	call := llm.Message{Role: llm.RoleAssistant, Content: llm.Content{
		llm.ToolUseBlock{ID: "t1", Name: "read", Input: json.RawMessage(`{}`)}}}
	result := llm.Message{Role: llm.RoleUser, Content: llm.Content{
		llm.ToolResultBlock{ToolUseID: "t1", Content: strings.Repeat("y", 8000)}}}

	history := []llm.Message{long(llm.RoleUser, 8000), call, result, long(llm.RoleAssistant, 100)}

	cut := cutPoint(history, 1000)
	if cut > 0 && opensWithToolResult(history[cut]) {
		t.Errorf("cut at %d leaves a tool result without its call", cut)
	}
}

func TestCutOfAShortConversationIsNothing(t *testing.T) {
	if cut := cutPoint(conversation(1, 10), 20_000); cut != 0 {
		t.Errorf("cut = %d, want nothing cut from a short conversation", cut)
	}
}

func TestCompactionFoldsAndKeeps(t *testing.T) {
	ag, api := newTestAgent(t, textTurn("They refactored the agent loop and it works."))
	ag.Compaction = &Compaction{Window: 40_000, KeepRecent: 2000}

	history := conversation(10, 4000)
	folded, err := ag.Fold(context.Background(), history)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}

	if len(folded) >= len(history) {
		t.Errorf("folded to %d messages from %d", len(folded), len(history))
	}
	if !strings.HasPrefix(folded[0].Text(), SummaryPrefix) {
		t.Errorf("first message = %q, want the summary", folded[0].Text())
	}
	if !strings.Contains(folded[0].Text(), "refactored the agent loop") {
		t.Errorf("summary = %q, want what the model wrote", folded[0].Text())
	}

	// The summariser must not be handed the tools or the conversation's system
	// prompt; it has one job.
	if req := api.request(0); len(req.Tools) != 0 {
		t.Errorf("the summariser was given %d tools", len(req.Tools))
	}
}

func TestCompactionTellsTheFrontend(t *testing.T) {
	ag, _ := newTestAgent(t, textTurn("a summary"))
	ag.Compaction = &Compaction{Window: 40_000, KeepRecent: 2000}

	var gotSummary string
	var gotKept int
	ag.Hooks.OnCompact = func(summary string, kept []llm.Message) {
		gotSummary, gotKept = summary, len(kept)
	}

	if _, err := ag.Fold(context.Background(), conversation(10, 4000)); err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if gotSummary != "a summary" {
		t.Errorf("hook got %q", gotSummary)
	}
	if gotKept == 0 {
		t.Error("hook was told nothing was kept")
	}
}

// Compaction failing should not take the conversation with it.
func TestAFailedSummaryLeavesTheHistoryAlone(t *testing.T) {
	ag, _ := newTestAgent(t, textTurn(""))
	ag.Compaction = &Compaction{Window: 40_000, KeepRecent: 2000}

	history := conversation(10, 4000)
	folded, err := ag.Fold(context.Background(), history)
	if err == nil {
		t.Fatal("an empty summary was accepted")
	}
	if len(folded) != len(history) {
		t.Errorf("history is %d messages after a failed compaction, want %d", len(folded), len(history))
	}
}

func TestSerialiseFlattensToolTraffic(t *testing.T) {
	history := []llm.Message{
		llm.UserText("read the agent"),
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ToolUseBlock{Name: "read", Input: json.RawMessage(`{"path":"agent.go"}`)}}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{Content: strings.Repeat("z", 5000)}}},
	}

	out := serialise(history)
	if !strings.Contains(out, "read the agent") || !strings.Contains(out, "called read") {
		t.Errorf("serialise lost the shape of the conversation:\n%s", out)
	}
	if len(out) > 2000 {
		t.Errorf("serialise passed on %d characters; tool output should be clipped", len(out))
	}
}
