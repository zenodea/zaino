package tui

import (
	"strings"
	"testing"

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
