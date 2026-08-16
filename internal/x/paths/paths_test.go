package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataRootsUnderXDG(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	got, err := Data("sessions", "abc.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "zaino", "sessions", "abc.jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(want)); err != nil {
		t.Errorf("parent directory was not created: %v", err)
	}
}

func TestDataFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)

	got, err := Data("wire.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "zaino", "wire.jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDataWithNoElementsIsTheRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	got, err := Data()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "zaino"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDataReportsAnUnusableRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", file)

	if _, err := Data("sessions", "x.jsonl"); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ dir, want string }{
		{"/Users/zeno/zaino", "Users-zeno-zaino"},
		{"/Users/zeno/zaino/", "Users-zeno-zaino"},
		{"/Users/zeno/./zaino", "Users-zeno-zaino"},
		{"relative/path", "relative-path"},
		{"/with space/and+plus", "with-space-and-plus"},
		{"/keeps-_.chars", "keeps-_.chars"},
		{"/", "root"},
		{"---", "root"},
	}
	for _, c := range cases {
		if got := Slug(c.dir); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

// Two workspaces must not collapse onto one another's saved sessions.
func TestSlugSeparatesSiblings(t *testing.T) {
	if Slug("/a/b") == Slug("/a/c") {
		t.Error("distinct directories produced the same slug")
	}
}
