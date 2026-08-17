package tui

import (
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func said(m *Model, texts ...string) {
	for i, text := range texts {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		m.messages = append(m.messages, llm.Message{Role: role, Content: llm.Content{llm.TextBlock{Text: text}}})
	}
}

func TestRewindTrimsTheContextAndHandsThePromptBack(t *testing.T) {
	m := newTestModel(t, 80, 24)
	said(m, "first thing", "an answer", "second thing", "another answer")

	m.runCommand("/rewind second")

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want the first exchange alone", len(m.messages))
	}
	if m.input.Value() != "second thing" {
		t.Errorf("composer = %q, want the prompt back to be changed", m.input.Value())
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "rewound") {
		t.Errorf("entry = %+v", last)
	}
}

// A tool result is not a turn: going back to one would leave its call
// without an answer.
func TestOnlyYourOwnPromptsAreTurns(t *testing.T) {
	m := newTestModel(t, 80, 24)
	said(m, "first thing")
	m.messages = append(m.messages,
		llm.Message{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ToolUseBlock{ID: "toolu_a", Name: "read"}}},
		llm.Message{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "toolu_a", Content: "a file"}}},
	)

	m.runCommand("/rewind")
	if len(m.chooser.options) != 1 {
		t.Errorf("%d turns offered, want the one prompt", len(m.chooser.options))
	}
}

func TestRewindWithNothingSaid(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.runCommand("/rewind")
	if len(m.messages) != 0 {
		t.Error("something was sent")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "nothing to go back to") {
		t.Errorf("entry = %+v", last)
	}
}

func TestRewindToSomethingNeverAsked(t *testing.T) {
	m := newTestModel(t, 80, 24)
	said(m, "first thing", "an answer")

	m.runCommand("/rewind nonsense")
	if len(m.messages) != 2 {
		t.Errorf("messages = %d, want the context left alone", len(m.messages))
	}
	if last := m.entries[len(m.entries)-1]; last.kind != entryError {
		t.Errorf("entry = %+v, want an error", last)
	}
}
