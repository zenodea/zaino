package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func blobs(t *testing.T) *Blobs {
	t.Helper()
	b, err := OpenBlobs(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBlobsAreContentAddressed(t *testing.T) {
	b := blobs(t)
	h1, err := b.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := b.Put([]byte("hello"))
	if h1 != h2 || h1 != Hash([]byte("hello")) {
		t.Errorf("hashes %s %s", h1, h2)
	}
	if got, _ := b.Get(h1); string(got) != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestKeepMarksWhatIsTooBigToHold(t *testing.T) {
	b := blobs(t)
	fc, err := b.Keep(Change{Path: "big", Existed: true, Before: make([]byte, MaxBlob+1), After: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	if !fc.Large || fc.Before == "" {
		t.Errorf("fc = %+v, want large with hashes kept", fc)
	}
	if _, err := b.Get(fc.Before); err == nil {
		t.Error("a large blob should not be stored")
	}
	fc, _ = b.Keep(Change{Path: "new", After: []byte("made")})
	if fc.Existed || fc.Before != "" || fc.After == "" {
		t.Errorf("created file = %+v", fc)
	}
}

// A tree with a fork: root → e1 (a.txt made) → e2 (a.txt edited, b.txt made)
//
//	└→ e3 (a.txt edited another way)
func fileTree(t *testing.T, b *Blobs) []Entry {
	t.Helper()
	keep := func(c Change) FileChange {
		fc, err := b.Keep(c)
		if err != nil {
			t.Fatal(err)
		}
		return fc
	}
	msg := llm.UserText("x")
	entry := func(id, parent string, files ...FileChange) Entry {
		return Entry{ID: id, Parent: parent, Type: KindMessage, body: body{Message: &msg, Files: files}}
	}
	return []Entry{
		entry("e1", "", keep(Change{Path: "a.txt", After: []byte("a1")})),
		entry("e2", "e1",
			keep(Change{Path: "a.txt", Existed: true, Before: []byte("a1"), After: []byte("a2")}),
			keep(Change{Path: "b.txt", After: []byte("b1")})),
		entry("e3", "e1", keep(Change{Path: "a.txt", Existed: true, Before: []byte("a1"), After: []byte("a3")})),
	}
}

func TestPlanFollowsTheTree(t *testing.T) {
	b := blobs(t)
	entries := fileTree(t, b)

	plan := PlanFiles(entries, "e2", "e1")
	if len(plan) != 2 {
		t.Fatalf("e2 → e1 plan = %+v", plan)
	}
	if plan[0].Path != "a.txt" || plan[0].Want != Hash([]byte("a1")) || plan[0].Expect != Hash([]byte("a2")) {
		t.Errorf("a.txt = %+v", plan[0])
	}
	if plan[1].Path != "b.txt" || plan[1].WantExists || !plan[1].ExpectExists {
		t.Errorf("b.txt = %+v, want it gone", plan[1])
	}

	plan = PlanFiles(entries, "e2", "e3")
	if len(plan) != 2 || plan[0].Want != Hash([]byte("a3")) || plan[1].WantExists {
		t.Errorf("e2 → e3 plan = %+v", plan)
	}

	plan = PlanFiles(entries, "e3", "")
	if len(plan) != 1 || plan[0].WantExists || plan[0].Expect != Hash([]byte("a3")) {
		t.Errorf("e3 → root plan = %+v, want a.txt removed", plan)
	}

	if plan := PlanFiles(entries, "e2", "e2"); len(plan) != 0 {
		t.Errorf("staying put plans %+v", plan)
	}
}

func TestRestoreWritesWhatThePlanSaysAndFlagsConflicts(t *testing.T) {
	b := blobs(t)
	entries := fileTree(t, b)
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "a2")
	write("b.txt", "b1")

	plan := PlanFiles(entries, "e2", "e3")
	if c := Conflicts(root, plan); len(c) != 0 {
		t.Fatalf("conflicts before anything changed: %v", c)
	}
	done, err := RestoreFiles(root, b, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(done.Files, []string{"a.txt", "b.txt"}) || len(done.Conflicts) != 0 {
		t.Errorf("restored = %+v", done)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(got) != "a3" {
		t.Errorf("a.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); !os.IsNotExist(err) {
		t.Error("b.txt should be gone at e3")
	}

	write("a.txt", "someone else's")
	plan = PlanFiles(entries, "e3", "e2")
	if c := Conflicts(root, plan); !slices.Equal(c, []string{"a.txt"}) {
		t.Errorf("conflicts = %v", c)
	}
	done, _ = RestoreFiles(root, b, plan, false)
	if !slices.Equal(done.Conflicts, []string{"a.txt"}) || !slices.Equal(done.Files, []string{"b.txt"}) {
		t.Errorf("restored with a conflict = %+v", done)
	}
	done, _ = RestoreFiles(root, b, plan, true)
	if got, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(got) != "a2" || !strings.Contains(strings.Join(done.Files, ","), "a.txt") {
		t.Errorf("forced: a.txt = %q, %+v", got, done)
	}
}
