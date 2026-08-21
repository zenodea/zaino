package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func TestLimitGateOpensOnTheLimitError(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.MaxContext = 200_000
	m.messages = []llm.Message{llm.UserText("hi")}

	send(m, doneMsg{
		Messages: m.messages,
		Err:      &agent.ContextLimitError{Used: 201_118, Limit: 200_000, Exact: true},
	})

	if !m.limit.open {
		t.Fatal("the limit gate did not open")
	}
	if m.streaming {
		t.Error("still streaming after the turn was held")
	}

	view := m.limitView()
	for _, want := range []string{"context limit", "201.1k", "200.0k", "send it anyway", "counted by anthropic"} {
		if !strings.Contains(view, want) {
			t.Errorf("the gate does not say %q:\n%s", want, view)
		}
	}
}

func TestLimitGateSaysWhenTheNumberIsEstimated(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m, doneMsg{Err: &agent.ContextLimitError{Used: 201_118, Limit: 200_000}})

	if view := m.limitView(); !strings.Contains(view, "about") || !strings.Contains(view, "estimated here") {
		t.Errorf("an estimate was presented as a count:\n%s", view)
	}
}

func TestLimitGateEscapeLeavesTheTurnUnsent(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.MaxContext = 200_000
	send(m,
		doneMsg{Err: &agent.ContextLimitError{Used: 201_118, Limit: 200_000, Exact: true}},
		key("esc"),
	)

	if m.limit.open {
		t.Error("the gate stayed open")
	}
	if m.streaming {
		t.Error("escape started the turn anyway")
	}
	if text := lastEntry(t, m).text; !strings.Contains(text, "not sent") {
		t.Errorf("last entry = %q, want it to say the turn was not sent", text)
	}
}

func TestLimitGateEnterSendsItAnyway(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.MaxContext = 200_000
	m.messages = []llm.Message{llm.UserText("hi")}

	send(m,
		doneMsg{Messages: m.messages, Err: &agent.ContextLimitError{Used: 201_118, Limit: 200_000, Exact: true}},
		key("enter"),
	)

	if m.limit.open {
		t.Error("the gate stayed open")
	}
	if !m.streaming {
		t.Error("enter did not send the turn")
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// The gate takes the keys before anything else can: a stray "y" must not slip
// into the composer while the turn is held.
func TestLimitGateSwallowsOtherKeys(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m,
		doneMsg{Err: &agent.ContextLimitError{Used: 201_118, Limit: 200_000, Exact: true}},
		key("y"),
	)

	if !m.limit.open {
		t.Error("an unrelated key closed the gate")
	}
	if m.input.Value() != "" {
		t.Errorf("the composer took %q while the gate was open", m.input.Value())
	}
}

func TestLimitCommandSetsAndClearsTheCeiling(t *testing.T) {
	m := newTestModel(t, 80, 24)

	cmdLimit(m, "200k")
	if m.agent.MaxContext != 200_000 {
		t.Errorf("MaxContext = %d, want 200000", m.agent.MaxContext)
	}

	cmdLimit(m, "off")
	if m.agent.MaxContext != 0 {
		t.Errorf("MaxContext = %d, want 0", m.agent.MaxContext)
	}

	cmdLimit(m, "lots")
	if e := lastEntry(t, m); e.kind != entryError {
		t.Errorf("a nonsense size was accepted: %+v", e)
	}
}

func TestLimitCommandIsRecordedInTheSession(t *testing.T) {
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)

	cmdLimit(m, "200k")
	cmdLimit(m, "off")

	var tokens []int
	for _, e := range storedEntries(t, repo, rec.ID()) {
		if e.Type == session.KindLimit && e.Tokens != nil {
			tokens = append(tokens, *e.Tokens)
		}
	}
	if len(tokens) != 2 || tokens[0] != 200_000 || tokens[1] != 0 {
		t.Errorf("recorded limits = %v, want [200000 0]", tokens)
	}
}

func storedEntries(t *testing.T, repo session.Repo, id string) []session.Entry {
	t.Helper()
	store, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	return entries
}

func TestTheTurnLimitHoldsAndCarriesOn(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.MaxTurns = 4
	m.streaming = true
	history := []llm.Message{llm.UserText("go"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.ToolUseBlock{ID: "t1", Name: "read"}}},
		{Role: llm.RoleUser, Content: llm.Content{llm.ToolResultBlock{ToolUseID: "t1", Content: "x"}}}}

	m.Update(doneMsg{Messages: history, Err: agent.ErrMaxTurns})

	if m.streaming {
		t.Fatal("the turn should be held, not running")
	}
	if !m.limit.open || !m.limit.turns || m.limit.limit != 4 {
		t.Fatalf("limit gate = %+v, want the turn limit held at 4", m.limit)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "turn limit") || !strings.Contains(view, "carry on") {
		t.Errorf("the panel should say what happened and offer to carry on:\n%s", view)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.streaming || m.limit.open {
		t.Error("⏎ should let it carry on")
	}
	if len(m.messages) != 3 {
		t.Errorf("messages = %d, want the history kept whole", len(m.messages))
	}
}

func TestLeavingTheTurnLimitKeepsTheHistory(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.Update(doneMsg{Messages: []llm.Message{llm.UserText("go")}, Err: agent.ErrMaxTurns})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.limit.open || m.streaming {
		t.Error("esc should leave the turn where it is")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "left there") {
		t.Errorf("entry = %+v", last)
	}
}
