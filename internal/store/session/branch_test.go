package session_test

import (
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func record(t *testing.T, store session.Store, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if _, err := store.Append(session.Message(llm.UserText(text), nil)); err != nil {
			t.Fatal(err)
		}
	}
}

func prompts(c session.Context) string {
	out := make([]string, len(c.Messages))
	for i, m := range c.Messages {
		out[i] = m.Text()
	}
	return strings.Join(out, ",")
}

func fresh(t *testing.T) session.Store {
	t.Helper()
	repo, err := session.OpenDir(t.TempDir(), "/work")
	if err != nil {
		t.Fatal(err)
	}
	store, err := repo.Create()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// The turns left behind stay in the file; what comes back is the line that
// leads to the newest entry.
func TestABranchLeavesTheOtherTurnsBehind(t *testing.T) {
	store := fresh(t)
	record(t, store, "one", "two", "three")

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetLeaf(entries[0].ID); err != nil {
		t.Fatal(err)
	}
	record(t, store, "instead")

	entries, err = store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Errorf("%d entries on disk, want every one of them kept", len(entries))
	}
	if got := prompts(session.Build(entries)); got != "one,instead" {
		t.Errorf("context = %q, want the branch that was taken", got)
	}
}

func TestRewindForksAtTheChosenTurn(t *testing.T) {
	store := fresh(t)
	rec := session.NewRecorder(store)

	messages := []llm.Message{llm.UserText("one"), llm.UserText("two"), llm.UserText("three")}
	if err := rec.Messages(messages); err != nil {
		t.Fatal(err)
	}

	if err := rec.Rewind(1); err != nil {
		t.Fatal(err)
	}
	if err := rec.Messages([]llm.Message{messages[0], llm.UserText("instead")}); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if got := prompts(session.Build(entries)); got != "one,instead" {
		t.Errorf("context = %q", got)
	}
}

// Rewinding then recording must not write the kept messages twice.
func TestRewindLeavesTheRecorderCounting(t *testing.T) {
	store := fresh(t)
	rec := session.NewRecorder(store)

	messages := []llm.Message{llm.UserText("one"), llm.UserText("two")}
	if err := rec.Messages(messages); err != nil {
		t.Fatal(err)
	}
	if err := rec.Rewind(1); err != nil {
		t.Fatal(err)
	}
	if err := rec.Messages([]llm.Message{messages[0], llm.UserText("instead")}); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("%d entries, want the two originals and the one new", len(entries))
	}
}

func TestRewindingPastTheEndDoesNothing(t *testing.T) {
	store := fresh(t)
	rec := session.NewRecorder(store)
	if err := rec.Messages([]llm.Message{llm.UserText("one")}); err != nil {
		t.Fatal(err)
	}

	if err := rec.Rewind(9); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if got := prompts(session.Build(entries)); got != "one" {
		t.Errorf("context = %q", got)
	}
}

func TestAFileThatNeverBranchedIsItsOwnPath(t *testing.T) {
	repo, id := write(t,
		`{"id":"a","seq":1,"at":"2026-08-17T17:03:13Z","type":"message","message":{"role":"user","content":[{"type":"text","text":"one"}]}}`,
		`{"id":"b","parent":"a","seq":2,"at":"2026-08-17T17:03:14Z","type":"message","message":{"role":"user","content":[{"type":"text","text":"two"}]}}`,
	)
	store, err := repo.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if got := prompts(session.Build(entries)); got != "one,two" {
		t.Errorf("context = %q", got)
	}
}

// The marks are what a rewind steers by, so they have to line up with the
// messages one for one.
func TestMarksLineUpWithTheMessages(t *testing.T) {
	store := fresh(t)
	record(t, store, "one", "two")

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	built := session.Build(entries)
	if len(built.Marks) != len(built.Messages) {
		t.Fatalf("%d marks for %d messages", len(built.Marks), len(built.Messages))
	}
	if built.Marks[0] != entries[0].ID || built.Marks[1] != entries[1].ID {
		t.Errorf("marks = %v, want the entries they came from", built.Marks)
	}
}
