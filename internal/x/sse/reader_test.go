package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func collect(t *testing.T, raw string) []string {
	t.Helper()
	r := NewReader(strings.NewReader(raw))
	var out []string
	for {
		ev, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("Next: %v", err)
			}
			return out
		}
		out = append(out, string(ev))
	}
}

func TestEventsAreSplitOnBlankLines(t *testing.T) {
	got := collect(t, "data: one\n\ndata: two\n\n")
	want := []string{"one", "two"}
	if len(got) != len(want) {
		t.Fatalf("got %d events %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSeveralDataLinesJoinWithNewlines(t *testing.T) {
	got := collect(t, "data: {\ndata:   \"a\": 1\ndata: }\n\n")
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if want := "{\n  \"a\": 1\n}"; got[0] != want {
		t.Errorf("got %q, want %q", got[0], want)
	}
}

func TestOnlyTheFirstSpaceAfterTheColonIsDropped(t *testing.T) {
	got := collect(t, "data:  padded\n\ndata:tight\n\n")
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0] != " padded" {
		t.Errorf("padded = %q, want %q", got[0], " padded")
	}
	if got[1] != "tight" {
		t.Errorf("tight = %q, want %q", got[1], "tight")
	}
}

func TestCommentsAndOtherFieldsAreIgnored(t *testing.T) {
	got := collect(t, ": keep-alive\nevent: message\nid: 7\ndata: payload\n\n")
	if len(got) != 1 || got[0] != "payload" {
		t.Fatalf("got %q, want [payload]", got)
	}
}

func TestCarriageReturnsAreStripped(t *testing.T) {
	got := collect(t, "data: one\r\n\r\ndata: two\r\n\r\n")
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %q, want [one two]", got)
	}
}

func TestBlankLinesWithoutDataEmitNothing(t *testing.T) {
	got := collect(t, "\n\n: ping\n\ndata: real\n\n\n")
	if len(got) != 1 || got[0] != "real" {
		t.Fatalf("got %q, want [real]", got)
	}
}

// A stream that ends without its final blank line still has one event in it.
func TestATrailingEventSurvivesEOF(t *testing.T) {
	got := collect(t, "data: one\n\ndata: last")
	if len(got) != 2 || got[1] != "last" {
		t.Fatalf("got %q, want [one last]", got)
	}
}

func TestEmptyInputIsJustEOF(t *testing.T) {
	if got := collect(t, ""); len(got) != 0 {
		t.Fatalf("got %q, want no events", got)
	}
}

func TestAnEmptyDataLineIsStillAnEvent(t *testing.T) {
	got := collect(t, "data:\n\n")
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("got %q, want one empty event", got)
	}
}

func TestReadErrorsAreReported(t *testing.T) {
	boom := errors.New("boom")
	r := NewReader(io.MultiReader(strings.NewReader("data: one\n\n"), errReader{boom}))
	if _, err := r.Next(); err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if _, err := r.Next(); !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
