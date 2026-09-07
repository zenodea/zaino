package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndEditReportWhatTheyChanged(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	call, err := (&Write{w}).Prepare(json.RawMessage(`{"path": "new.txt", "content": "one\n"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	changes := call.(Changed).Changes()
	if len(changes) != 1 || changes[0].Existed || changes[0].Path != "new.txt" || string(changes[0].After) != "one\n" {
		t.Errorf("write changes = %+v", changes)
	}

	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.MarkRead(filepath.Join(root, "new.txt"), []byte("one\n"))
	call, err = (&Edit{w}).Prepare(json.RawMessage(`{"path": "new.txt", "old_text": "one", "new_text": "two"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	changes = call.(Changed).Changes()
	if len(changes) != 1 || !changes[0].Existed || string(changes[0].Before) != "one\n" || string(changes[0].After) != "two\n" {
		t.Errorf("edit changes = %+v", changes)
	}
}

func TestNothingWritesInsideDotGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	call, err := (&Write{w}).Prepare(json.RawMessage(`{"path": ".git/HEAD", "content": "ref: refs/heads/evil"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call.Run(context.Background()); err == nil || !strings.Contains(err.Error(), ".git") {
		t.Errorf("err = %v, want a refusal naming .git", err)
	}
}

func TestRootCallIDIsTheOutermost(t *testing.T) {
	ctx := WithRootCallID(context.Background(), "outer")
	ctx = WithRootCallID(ctx, "inner")
	if got := RootCallID(ctx); got != "outer" {
		t.Errorf("root = %q", got)
	}
}
