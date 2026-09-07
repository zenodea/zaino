package last

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenodea/zaino/internal/store/session"
)

func open(t *testing.T, project string) *Store {
	t.Helper()
	s, err := Open(project)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func str(s string) *string { return &s }

func TestNothingYetIsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	got, err := open(t, "/work/a").Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.empty() || got.From != "" {
		t.Errorf("got %+v, want nothing", got)
	}
}

func TestAProjectRemembersItsOwnPicks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	a := open(t, "/work/a")

	if err := a.Remember(Settings{Provider: "anthropic", Model: str("claude-opus-5")}); err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(Settings{Effort: str("high")}); err != nil {
		t.Fatal(err)
	}

	got, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "anthropic" || got.Model == nil || *got.Model != "claude-opus-5" {
		t.Errorf("provider/model = %q/%v, want the picks", got.Provider, got.Model)
	}
	if got.Effort == nil || *got.Effort != "high" {
		t.Errorf("effort = %v, want the later pick to add to the earlier", got.Effort)
	}
	if want := filepath.Join(root, "zaino", "last.json"); got.From != want {
		t.Errorf("from = %q, want %q", got.From, want)
	}
}

func TestANewProjectStartsFromTheMostRecentPickAnywhere(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := open(t, "/work/a").Remember(Settings{Provider: "openai", Model: str("gpt-x")}); err != nil {
		t.Fatal(err)
	}

	got, err := open(t, "/work/b").Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want a's pick to carry over", got.Provider)
	}
}

func TestAProjectWithItsOwnMemoryIgnoresLaterPicksElsewhere(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a, b := open(t, "/work/a"), open(t, "/work/b")

	if err := a.Remember(Settings{Effort: str("max")}); err != nil {
		t.Fatal(err)
	}
	if err := b.Remember(Settings{Effort: str("low")}); err != nil {
		t.Fatal(err)
	}

	got, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort == nil || *got.Effort != "max" {
		t.Errorf("a's effort = %v, want its own", got.Effort)
	}
}

func TestOutsideAnyProjectItIsAllRecent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	none := open(t, "")
	if err := none.Remember(Settings{Effort: str("high")}); err != nil {
		t.Fatal(err)
	}

	got, err := none.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort == nil || *got.Effort != "high" {
		t.Errorf("effort = %v, want the pick", got.Effort)
	}
	raw, err := os.ReadFile(none.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || filepath.Base(none.Path()) != "last.json" {
		t.Errorf("wrote %q to %q", raw, none.Path())
	}
	if got, _ := open(t, "/work/c").Load(); got.Effort == nil || *got.Effort != "high" {
		t.Errorf("a project = %v, want the pick made outside any", got.Effort)
	}
}

// Picking the provider's default is a choice, and comes back as one.
func TestBackToDefaultIsRememberedAsSuch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := open(t, "/work/a")
	if err := a.Remember(Settings{Effort: str("high")}); err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(Settings{Effort: str("")}); err != nil {
		t.Fatal(err)
	}
	got, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort == nil || *got.Effort != "" {
		t.Errorf("effort = %v, want an explicit default", got.Effort)
	}
}

func TestOf(t *testing.T) {
	s, ok := Of(session.Model("anthropic", ""))
	if !ok || s.Provider != "anthropic" || s.Model == nil || *s.Model != "" {
		t.Errorf("model entry → %+v", s)
	}
	s, ok = Of(session.Effort("xhigh"))
	if !ok || s.Effort == nil || *s.Effort != "xhigh" || s.Provider != "" {
		t.Errorf("effort entry → %+v", s)
	}
	if _, ok := Of(session.Clear()); ok {
		t.Error("a clear is not a setting")
	}
}

func TestAMangledFileIsAnError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	s := open(t, "/work/a")
	if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Error("load: want an error naming the file")
	}
}
