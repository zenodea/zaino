package session

import (
	"path/filepath"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestChangesLandOnTheMessageThatCarriesTheResult(t *testing.T) {
	repo, err := OpenDir(filepath.Join(t.TempDir(), "sessions"), "/work")
	if err != nil {
		t.Fatal(err)
	}
	store, err := repo.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store)
	rec.UseBlobs(blobs(t))

	if err := rec.NoteChange("toolu_a", Change{Path: "a.txt", After: []byte("made")}); err != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{
		llm.UserText("make a.txt"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.ToolUseBlock{ID: "toolu_a", Name: "write"}}},
		{Role: llm.RoleUser, Content: llm.Content{llm.ToolResultBlock{ToolUseID: "toolu_a", Content: "Created a.txt"}}},
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "done"}}},
	}
	if err := rec.Messages(messages); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	var carrying []int
	for i, e := range entries {
		if len(e.Files) > 0 {
			carrying = append(carrying, i)
		}
	}
	if len(carrying) != 1 || carrying[0] != 2 {
		t.Fatalf("files on entries %v, want only the tool-result message", carrying)
	}
	fc := entries[2].Files[0]
	if fc.Path != "a.txt" || fc.Existed || fc.After != Hash([]byte("made")) {
		t.Errorf("recorded %+v", fc)
	}
}

func TestChangesAreDroppedWithoutBlobs(t *testing.T) {
	rec := NewRecorder(nil)
	if err := rec.NoteChange("x", Change{Path: "a"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.changes) != 0 {
		t.Error("noted a change with nowhere to keep it")
	}
}
