package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryKeepsNotesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes", "proj.md")
	m := NewMemory(path)

	if got, err := runTool(t, m, `{}`); err != nil || !strings.Contains(got, "nothing noted") {
		t.Errorf("empty list = %q, %v", got, err)
	}
	if _, err := runTool(t, m, `{"action": "add", "text": "tests run with go test ./..."}`); err != nil {
		t.Fatal(err)
	}
	if got, _ := runTool(t, m, `{"action": "add", "text": "Tests run with go test ./..."}`); got != "already noted" {
		t.Errorf("duplicate = %q", got)
	}
	if _, err := runTool(t, m, `{"action": "add", "text": "the user likes short commits"}`); err != nil {
		t.Fatal(err)
	}

	again := NewMemory(path)
	if notes := again.Notes(); len(notes) != 2 || notes[0] != "tests run with go test ./..." {
		t.Errorf("notes = %q", notes)
	}
	if got, _ := runTool(t, again, `{"action": "forget", "text": "commits"}`); !strings.HasPrefix(got, "forgot 1") {
		t.Errorf("forget = %q", got)
	}
	if got, _ := runTool(t, again, `{"action": "forget", "text": "commits"}`); !strings.Contains(got, "nothing") {
		t.Errorf("forget again = %q", got)
	}
	if got, _ := runTool(t, again, `{"action": "list"}`); got != "- tests run with go test ./..." {
		t.Errorf("list = %q", got)
	}

	if _, err := runTool(t, again, `{"action": "forget", "text": "go test"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty memory should leave no file behind")
	}
}

func TestMemoryRefusesWhatItCannotDo(t *testing.T) {
	m := NewMemory(filepath.Join(t.TempDir(), "m.md"))
	for _, input := range []string{`{"action": "add"}`, `{"action": "forget", "text": ""}`, `{"action": "eat"}`} {
		if _, err := m.Prepare([]byte(input)); err == nil {
			t.Errorf("%s: want an error", input)
		}
	}
}
