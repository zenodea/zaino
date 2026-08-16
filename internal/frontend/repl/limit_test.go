package repl

import (
	"bufio"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/agent"
)

func TestAskPastLimit(t *testing.T) {
	err := &agent.ContextLimitError{Used: 201_118, Limit: 200_000, Exact: true}

	for _, tc := range []struct {
		name  string
		typed string
		want  bool
	}{
		{name: "bare enter sends it", typed: "\n", want: true},
		{name: "spaces are still a bare enter", typed: "   \n", want: true},
		{name: "anything typed leaves it", typed: "no\n"},
		{name: "eof leaves it", typed: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := askPastLimit(err, bufio.NewReader(strings.NewReader(tc.typed))); got != tc.want {
				t.Errorf("askPastLimit(%q) = %v, want %v", tc.typed, got, tc.want)
			}
		})
	}
}

// Piped input has nobody to ask, so the ceiling holds.
func TestAskPastLimitWithoutATerminal(t *testing.T) {
	if askPastLimit(&agent.ContextLimitError{Used: 201_118, Limit: 200_000}, nil) {
		t.Error("the limit was waived with no one to waive it")
	}
}
