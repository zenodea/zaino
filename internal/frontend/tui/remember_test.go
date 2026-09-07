package tui

import (
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/last"
	"github.com/zenodea/zaino/internal/store/session"
)

func rememberingModel(t *testing.T) (*Model, *last.Store) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := last.Open("/work/a")
	if err != nil {
		t.Fatal(err)
	}
	m := chooserModel(t)
	m.UseRemembered(store)
	return m, store
}

// What /model and /effort set is where the next run in this project starts.
func TestPicksAtThePromptAreRemembered(t *testing.T) {
	m, store := rememberingModel(t)

	m.runCommand("/model claude-sonnet-5")
	m.runCommand("/effort " + llm.EffortLow)

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "anthropic" || got.Model == nil || *got.Model != "claude-sonnet-5" {
		t.Errorf("provider/model = %q/%v, want the /model pick with its provider", got.Provider, got.Model)
	}
	if got.Effort == nil || *got.Effort != llm.EffortLow {
		t.Errorf("effort = %v, want the /effort pick", got.Effort)
	}
}

// Coming back to a session, or rewinding one, sets the agent to what the
// session had — that is a restore, not a pick, and leaves the memory alone.
func TestARestoreIsNotAPick(t *testing.T) {
	m, store := rememberingModel(t)

	m.applyContext(session.Context{Model: "claude-haiku-4-5", Effort: llm.EffortMax})

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != nil || got.Effort != nil {
		t.Errorf("remembered %+v after a restore, want nothing", got)
	}
	if m.agent.Model != "claude-haiku-4-5" {
		t.Errorf("model = %q, the restore itself should still land", m.agent.Model)
	}
}
