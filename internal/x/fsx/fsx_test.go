package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicWritesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	if err := WriteAtomic(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "first" {
		t.Errorf("got %q, want %q", got, "first")
	}

	if err := WriteAtomic(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

func TestWriteAtomicLeavesNoTempBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteAtomic(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Errorf("directory holds %v, want just state.json", names(entries))
	}
}

func TestWriteAtomicHonoursPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := WriteAtomic(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}

func TestWriteAtomicFailsWithoutADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "state.json")
	if err := WriteAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("got nil, want an error")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("a temp file was left behind")
	}
}

func TestAppendLineCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "log.jsonl")
	if err := AppendLine(path, `{"n":1}`); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "{\"n\":1}\n" {
		t.Errorf("got %q", got)
	}
}

func TestAppendLineKeepsOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	for _, line := range []string{"one", "two", "three"} {
		if err := AppendLine(path, line); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := read(t, path), "one\ntwo\nthree\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendLineFailsOnAFileInThePath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendLine(filepath.Join(blocker, "log.jsonl"), "x"); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func names(entries []os.DirEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}

func TestWriteAtomicFailsWhenTheTargetIsADirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "occupied")
	if err := os.MkdirAll(filepath.Join(dir, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(dir, []byte("x"), 0o600); err == nil {
		t.Fatal("got nil, want an error")
	}
	if _, err := os.Stat(dir + ".tmp"); !os.IsNotExist(err) {
		t.Error("a temp file was left behind")
	}
}
