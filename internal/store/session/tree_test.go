package session_test

import (
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

// fork records three turns, takes the first back, and asks differently, so
// the file holds one root with two branches.
func fork(t *testing.T) (session.Store, []session.Entry) {
	t.Helper()
	store := fresh(t)
	record(t, store, "one", "two")

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
	return store, entries
}

func TestPathToReadsABranchLeftBehind(t *testing.T) {
	_, entries := fork(t)

	if got := prompts(session.BuildAt(entries, entries[1].ID)); got != "one,two" {
		t.Errorf("abandoned branch = %q, want it read back whole", got)
	}
	if got := prompts(session.Build(entries)); got != "one,instead" {
		t.Errorf("newest branch = %q", got)
	}
}

func TestPathToTheRootIsNoPathAtAll(t *testing.T) {
	_, entries := fork(t)

	if got := session.PathTo(entries, ""); got != nil {
		t.Errorf("path to the root = %v, want nothing", got)
	}
	if got := prompts(session.BuildAt(entries, "")); got != "" {
		t.Errorf("context at the root = %q, want it empty", got)
	}
}

func TestTreeGathersEveryBranch(t *testing.T) {
	_, entries := fork(t)

	roots := session.Tree(entries)
	if len(roots) != 1 {
		t.Fatalf("%d roots, want the one first turn", len(roots))
	}
	root := roots[0]
	if root.Entry.Prompt() != "one" {
		t.Errorf("root = %q", root.Entry.Prompt())
	}
	if len(root.Children) != 2 ||
		root.Children[0].Entry.Prompt() != "two" ||
		root.Children[1].Entry.Prompt() != "instead" {
		t.Errorf("children = %+v, want both branches oldest first", root.Children)
	}
}

// A tool result answers a call, and a summary is a boundary: neither is a
// turn the tree should offer.
func TestTreeSkipsWhatIsNotATurn(t *testing.T) {
	store := fresh(t)
	record(t, store, "one")
	for _, m := range []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ToolUseBlock{ID: "toolu_a", Name: "read"}}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "toolu_a", Content: "a file"}}},
		llm.UserText(session.SummaryPrefix + "what happened before"),
	} {
		if _, err := store.Append(session.Message(m, nil)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	roots := session.Tree(entries)
	if len(roots) != 1 || len(roots[0].Children) != 0 {
		t.Errorf("tree = %+v, want the one prompt alone", roots)
	}
}

func TestJumpRecordsOntoTheChosenBranch(t *testing.T) {
	store := fresh(t)
	rec := session.NewRecorder(store)

	messages := []llm.Message{llm.UserText("one"), llm.UserText("two")}
	if err := rec.Messages(messages); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Jump(entries[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := rec.Messages([]llm.Message{messages[0], llm.UserText("instead")}); err != nil {
		t.Fatal(err)
	}

	entries, err = store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("%d entries, want the two originals and the one new", len(entries))
	}
	if got := prompts(session.Build(entries)); got != "one,instead" {
		t.Errorf("context = %q", got)
	}
}
