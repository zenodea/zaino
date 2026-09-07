package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func editedTurn(t *testing.T, m *Model, prompt, id, path, before, after string) {
	t.Helper()
	existed := before != ""
	if err := m.rec.NoteChange(id, session.Change{Path: path, Existed: existed, Before: []byte(before), After: []byte(after)}); err != nil {
		t.Fatal(err)
	}
	m.messages = append(m.messages,
		llm.UserText(prompt),
		llm.Message{Role: llm.RoleAssistant, Content: llm.Content{llm.ToolUseBlock{ID: id, Name: "write"}}},
		llm.Message{Role: llm.RoleUser, Content: llm.Content{llm.ToolResultBlock{ToolUseID: id, Content: "ok"}}},
		llm.Message{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "done"}}},
	)
	if err := os.WriteFile(filepath.Join(m.root, path), []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.rec.Messages(m.messages); err != nil {
		t.Fatal(err)
	}
}

func checkpointed(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	blobs, err := session.OpenBlobs(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	m.UseCheckpoints(blobs, t.TempDir())
	m.UseSession(repo, rec)
	return m
}

func TestRewindOffersToPutTheFilesBack(t *testing.T) {
	m := checkpointed(t)
	editedTurn(t, m, "make it", "toolu_a", "a.txt", "", "v1")
	editedTurn(t, m, "change it", "toolu_b", "a.txt", "v1", "v2")

	m.runCommand("/rewind")
	if !m.chooser.open || !strings.Contains(m.chooser.title, "files") {
		t.Fatalf("no files chooser after rewind: %+v", m.chooser)
	}
	if len(m.chooser.options) != 2 {
		t.Errorf("options = %d, want restore and keep with nothing changed outside", len(m.chooser.options))
	}
	m.chooser.apply(m, m.chooser.options[0])

	if got, _ := os.ReadFile(filepath.Join(m.root, "a.txt")); string(got) != "v1" {
		t.Errorf("a.txt = %q, want the version before the second turn", got)
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "1 files put back") {
		t.Errorf("notice = %q", last.text)
	}

	m.runCommand("/rewind")
	m.chooser.apply(m, m.chooser.options[0])
	if _, err := os.Stat(filepath.Join(m.root, "a.txt")); !os.IsNotExist(err) {
		t.Error("a.txt should be gone at the start")
	}
}

func TestAFileChangedOutsideIsNotClobbered(t *testing.T) {
	m := checkpointed(t)
	editedTurn(t, m, "make it", "toolu_a", "a.txt", "", "v1")
	if err := os.WriteFile(filepath.Join(m.root, "a.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.runCommand("/rewind")
	if len(m.chooser.options) != 3 {
		t.Fatalf("options = %d, want an overwrite choice for the conflict", len(m.chooser.options))
	}
	m.chooser.apply(m, m.chooser.options[0])
	if got, _ := os.ReadFile(filepath.Join(m.root, "a.txt")); string(got) != "mine" {
		t.Errorf("a.txt = %q, want it left alone", got)
	}
}

func TestKeepingTheFilesLeavesThem(t *testing.T) {
	m := checkpointed(t)
	editedTurn(t, m, "make it", "toolu_a", "a.txt", "", "v1")

	m.runCommand("/rewind")
	m.chooser.apply(m, m.chooser.options[1])
	if got, _ := os.ReadFile(filepath.Join(m.root, "a.txt")); string(got) != "v1" {
		t.Errorf("a.txt = %q", got)
	}
}

func TestNoChooserWithCheckpointsOff(t *testing.T) {
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)
	m.root = t.TempDir()
	editedTurn(t, m, "make it", "toolu_a", "a.txt", "", "v1")

	m.runCommand("/rewind")
	if m.chooser.open {
		t.Error("a chooser opened with checkpoints off")
	}
}
