package repl

import (
	"testing"

	"github.com/zenodea/zaino/internal/store/last"
)

// What /model and /effort set is where the next run in this project starts.
func TestPicksAtThePromptAreRemembered(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := last.Open("/work/a")
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Remembered: store}
	ag := newAgent()

	run(t, ag, "/model stub-2", o)
	run(t, ag, "/effort high", o)

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "stub" || got.Model == nil || *got.Model != "stub-2" {
		t.Errorf("provider/model = %q/%v, want the /model pick with its provider", got.Provider, got.Model)
	}
	if got.Effort == nil || *got.Effort != "high" {
		t.Errorf("effort = %v, want the /effort pick", got.Effort)
	}

	run(t, ag, "/effort -", o)
	if got, _ := store.Load(); got.Effort == nil || *got.Effort != "" {
		t.Errorf("effort = %v, want going back to default remembered as a pick", got.Effort)
	}
}

func TestNothingToRememberIntoIsFine(t *testing.T) {
	run(t, newAgent(), "/effort high", Options{})
}
