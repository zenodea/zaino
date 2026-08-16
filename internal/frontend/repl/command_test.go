package repl

import (
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/provider"
)

func TestIsCommand(t *testing.T) {
	cases := map[string]bool{
		"/help":                  true,
		"/quit":                  true,
		"/accept-edits":          true,
		"/set_mode":              true,
		"/model claude-opus-5":   true,
		"/m2":                    true,
		"":                       false,
		"/":                      false,
		"/ help":                 false,
		"help":                   false,
		"//comment":              false,
		"/etc/hosts is wrong":    false,
		"/tmp/x.go needs a look": false,
		" /help":                 false,
		"what does /help do?":    false,
		"/re-run! the thing":     false,
	}
	for line, want := range cases {
		if got := isCommand(line); got != want {
			t.Errorf("isCommand(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestEveryDocumentedCommandIsRecognised(t *testing.T) {
	for _, line := range strings.Split(help, "\n") {
		name, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if !isCommand(name) {
			t.Errorf("/help lists %q but isCommand rejects it", name)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"a bit too long", 10, "a bit too…"},
		{"", 5, ""},
		{"abc", 0, "…"},
		{"abc", 1, "…"},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.limit); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.limit, got, c.want)
		}
	}
}

// Session previews are user text, so the limit has to be counted in runes.
func TestTruncateCountsRunesNotBytes(t *testing.T) {
	in := "héllo wörld ünicode"
	got := truncate(in, 8)
	if n := len([]rune(got)); n != 8 {
		t.Errorf("truncate(%q, 8) = %q (%d runes), want 8 runes", in, got, n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("got %q, want an ellipsis", got)
	}
}

func TestOrNone(t *testing.T) {
	if got := orNone("", "(none)"); got != "(none)" {
		t.Errorf("got %q, want (none)", got)
	}
	if got := orNone("set", "(none)"); got != "set" {
		t.Errorf("got %q, want set", got)
	}
}

func TestShownHidden(t *testing.T) {
	if got := shownHidden(true); got != "shown" {
		t.Errorf("got %q, want shown", got)
	}
	if got := shownHidden(false); got != "hidden" {
		t.Errorf("got %q, want hidden", got)
	}
}

func TestEffortsMatchWhatHelpPromises(t *testing.T) {
	if len(efforts) != 5 {
		t.Fatalf("got %d efforts, want 5", len(efforts))
	}
	for _, e := range efforts {
		if e == "" {
			t.Error("an effort level is empty")
		}
	}
}

func TestSetupHintCoversEveryProvider(t *testing.T) {
	for _, name := range provider.Available() {
		hint := setupHint(name)
		if env := credentialEnv[name]; env == "" {
			t.Errorf("%s has no credential variable recorded", name)
		} else if !strings.Contains(hint, env) {
			t.Errorf("the %s hint does not name %s:\n%s", name, env, hint)
		}
	}
}

// Only Anthropic has a login flow; the others must not be told to run one.
func TestOnlyAnthropicIsOfferedALogin(t *testing.T) {
	if !strings.Contains(setupHint("anthropic"), "ant auth login") {
		t.Errorf("got:\n%s", setupHint("anthropic"))
	}
	for _, name := range []string{"gemini", "openai", "grok"} {
		if strings.Contains(setupHint(name), "auth login") {
			t.Errorf("%s was offered a login it does not have:\n%s", name, setupHint(name))
		}
	}
}
