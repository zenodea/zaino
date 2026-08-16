package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/permission"
)

func approve(t *testing.T, typed string, req permission.Request) (permission.Grant, error) {
	t.Helper()
	saved := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	defer func() { os.Stderr = saved; devnull.Close() }()

	a := &approver{in: bufio.NewReader(strings.NewReader(typed))}
	return a.Approve(context.Background(), req)
}

func TestApproverReadsTheAnswer(t *testing.T) {
	cases := map[string]permission.Grant{
		"y\n":      permission.Once,
		"yes\n":    permission.Once,
		"Y\n":      permission.Once,
		"\n":       permission.Once,
		"  \n":     permission.Once,
		"a\n":      permission.Always,
		"always\n": permission.Always,
		"n\n":      permission.Reject,
		"no\n":     permission.Reject,
		"NO\n":     permission.Reject,
	}
	for typed, want := range cases {
		got, err := approve(t, typed, permission.Request{Tool: "write", Action: permission.Write, Target: "x.go"})
		if err != nil {
			t.Errorf("%q: %v", typed, err)
			continue
		}
		if got != want {
			t.Errorf("typing %q gave %v, want %v", typed, got, want)
		}
	}
}

func TestApproverAsksAgainAfterNonsense(t *testing.T) {
	got, err := approve(t, "maybe\nwhat\na\n", permission.Request{Tool: "bash", Action: permission.Execute, Target: "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if got != permission.Always {
		t.Errorf("got %v, want Always", got)
	}
}

// A closed stdin must not spin; it means no.
func TestApproverRefusesOnClosedInput(t *testing.T) {
	got, err := approve(t, "", permission.Request{Tool: "write", Action: permission.Write, Target: "x.go"})
	if err != nil {
		t.Fatal(err)
	}
	if got != permission.Reject {
		t.Errorf("got %v, want Reject", got)
	}
}

func TestApproverStopsWhenTheContextIsCancelled(t *testing.T) {
	saved := os.Stderr
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devnull
	defer func() { os.Stderr = saved; devnull.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &approver{in: bufio.NewReader(strings.NewReader("y\n"))}
	got, err := a.Approve(ctx, permission.Request{Tool: "write", Action: permission.Write})
	if err == nil {
		t.Fatal("got nil, want the context error")
	}
	if got != permission.Reject {
		t.Errorf("got %v, want Reject", got)
	}
}

func TestDecisionName(t *testing.T) {
	cases := map[permission.Grant]string{
		permission.Once:   "allowed",
		permission.Always: "allowed-always",
		permission.Reject: "refused",
	}
	for grant, want := range cases {
		if got := decisionName(grant); got != want {
			t.Errorf("decisionName(%v) = %q, want %q", grant, got, want)
		}
	}
}

func TestVerbNamesTheAction(t *testing.T) {
	cases := map[permission.Action]string{
		permission.Write:             "Write",
		permission.Execute:           "Run",
		permission.Network:           "Fetch",
		permission.Read:              "Read",
		permission.Action("unknown"): "Read",
	}
	for action, want := range cases {
		if got := verb(action); got != want {
			t.Errorf("verb(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestIndentShiftsEveryLine(t *testing.T) {
	got := indent("one\ntwo\n")
	if want := "  one\n  two"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIndentFoldsALongPreview(t *testing.T) {
	var lines []string
	for i := range 50 {
		lines = append(lines, string(rune('a'+i%26)))
	}
	got := indent(strings.Join(lines, "\n"))

	if n := strings.Count(got, "\n") + 1; n != 21 {
		t.Errorf("got %d lines, want 20 plus the summary", n)
	}
	if !strings.Contains(got, "30 more lines") {
		t.Errorf("the fold does not say how much is hidden:\n%s", got)
	}
}

func TestCompactJSON(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "{}"},
		{`{"a": 1,  "b":  2}`, `{"a":1,"b":2}`},
		{`{"a":1}`, `{"a":1}`},
		{`not json`, `not json`},
	}
	for _, c := range cases {
		if got := compactJSON(json.RawMessage(c.in)); got != c.want {
			t.Errorf("compactJSON(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The last answer in a piped script has no trailing newline; it still counts.
func TestApproverAcceptsALastAnswerWithoutANewline(t *testing.T) {
	got, err := approve(t, "a", permission.Request{Tool: "bash", Action: permission.Execute, Target: "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if got != permission.Always {
		t.Errorf("got %v, want Always", got)
	}
}
