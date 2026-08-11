package recall

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSkipsBlanksAndRepeats(t *testing.T) {
	l := New()
	for _, line := range []string{"one", "one", "  ", "two", "one"} {
		if err := l.Add(line); err != nil {
			t.Fatalf("Add(%q): %v", line, err)
		}
	}

	got := strings.Join(l.Lines(), ",")
	if got != "one,two,one" {
		t.Errorf("lines = %q, want %q", got, "one,two,one")
	}
}

func TestBrowseWalksBackAndRestoresDraft(t *testing.T) {
	l := New()
	l.Add("first")
	l.Add("second")

	if l.Browsing() {
		t.Error("browsing before any ↑")
	}

	line, ok := l.Prev("half-written")
	if !ok || line != "second" {
		t.Fatalf("first Prev = %q, %v; want \"second\", true", line, ok)
	}
	if !l.Browsing() {
		t.Error("not browsing after ↑")
	}

	if line, ok = l.Prev(""); !ok || line != "first" {
		t.Fatalf("second Prev = %q, %v; want \"first\", true", line, ok)
	}
	if _, ok = l.Prev(""); ok {
		t.Error("Prev past the oldest line succeeded")
	}

	if line, ok = l.Next(); !ok || line != "second" {
		t.Fatalf("Next = %q, %v; want \"second\", true", line, ok)
	}
	if line, ok = l.Next(); !ok || line != "half-written" {
		t.Fatalf("Next at the newest = %q, %v; want the draft back", line, ok)
	}
	if l.Browsing() {
		t.Error("still browsing after walking back to the draft")
	}
	if _, ok = l.Next(); ok {
		t.Error("Next while not browsing succeeded")
	}
}

func TestAddStopsBrowsing(t *testing.T) {
	l := New()
	l.Add("first")
	l.Prev("")

	l.Add("second")
	if l.Browsing() {
		t.Error("still browsing after Add")
	}
	if line, ok := l.Prev(""); !ok || line != "second" {
		t.Errorf("Prev after Add = %q, %v; want the newest line", line, ok)
	}
}

func TestRoundTripThroughFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recall")

	first, err := Load(path, DefaultLimit)
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	for _, line := range []string{"plain", "two\nlines", `back\slash`} {
		if err := first.Add(line); err != nil {
			t.Fatalf("Add(%q): %v", line, err)
		}
	}

	second, err := Load(path, DefaultLimit)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{`back\slash`, "two\nlines", "plain"}
	if got := second.Lines(); len(got) != len(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	for i, line := range want {
		if second.Lines()[i] != line {
			t.Errorf("line %d = %q, want %q", i, second.Lines()[i], line)
		}
	}
}

func TestLoadTrimsPastTheLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recall")

	l, _ := Load(path, 3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		l.Add(line)
	}

	reloaded, err := Load(path, 3)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(reloaded.Lines(), ","); got != "e,d,c" {
		t.Errorf("lines = %q, want %q", got, "e,d,c")
	}

	again, _ := Load(path, 3)
	if got := strings.Join(again.Lines(), ","); got != "e,d,c" {
		t.Errorf("after rewrite, lines = %q, want %q", got, "e,d,c")
	}
}
