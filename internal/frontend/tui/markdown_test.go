package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func hasAttr(s, attr string) bool {
	return strings.Contains(s, "\x1b["+attr+"m") || strings.Contains(s, "\x1b["+attr+";")
}

func md(t *testing.T, text string, width int) string {
	t.Helper()
	return renderMarkdown(text, width, bodyStyle)
}

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	m.Run()
}

func TestBoldAndItalic(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		plain string
		attr  string
	}{
		{"bold", "a **strong** word", "a strong word", "1"},
		{"bold underscores", "a __strong__ word", "a strong word", "1"},
		{"italic star", "a *slanted* word", "a slanted word", "3"},
		{"italic underscore", "a _slanted_ word", "a slanted word", "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := md(t, tt.in, 40)
			if plain := stripANSI(got); plain != tt.plain {
				t.Errorf("text = %q, want %q — the markers should be gone", plain, tt.plain)
			}
			if !hasAttr(got, tt.attr) {
				t.Errorf("output carries no styling: %q", got)
			}
		})
	}
}

func TestUnderscoresInsideWordsAreLeftAlone(t *testing.T) {
	got := md(t, "pass max_tokens and cache_read_tokens", 60)
	if plain := stripANSI(got); plain != "pass max_tokens and cache_read_tokens" {
		t.Errorf("text = %q, want the underscores kept", plain)
	}
	if hasAttr(got, "3") {
		t.Errorf("output italicised an identifier: %q", got)
	}
}

func TestInlineCode(t *testing.T) {
	got := md(t, "call `agent.Run(ctx)` first", 40)
	if plain := stripANSI(got); plain != "call agent.Run(ctx) first" {
		t.Errorf("text = %q", plain)
	}
}

func TestCodeBlockIsNotWrappedOrParsed(t *testing.T) {
	in := "text\n\n```go\nif a && b { // **not bold**\n}\n```"
	plain := stripANSI(md(t, in, 40))

	if !strings.Contains(plain, "**not bold**") {
		t.Errorf("code block lost its literal markers:\n%s", plain)
	}
	if !strings.Contains(plain, "  if a && b") {
		t.Errorf("code block lost its indent:\n%s", plain)
	}
	if strings.Contains(plain, "```") {
		t.Errorf("fence leaked into the output:\n%s", plain)
	}
}

func TestHeadingsAndLists(t *testing.T) {
	plain := stripANSI(md(t, "# Title\n\n- one\n- two\n\n1. first\n2. second", 40))

	if strings.Contains(plain, "#") {
		t.Errorf("heading marker leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "Title") {
		t.Errorf("heading text missing:\n%s", plain)
	}
	if !strings.Contains(plain, "• one") || !strings.Contains(plain, "• two") {
		t.Errorf("bullets missing:\n%s", plain)
	}
	if !strings.Contains(plain, "1. first") {
		t.Errorf("numbered list missing:\n%s", plain)
	}
}

func TestStyleSurvivesAWrap(t *testing.T) {
	got := md(t, "**"+strings.Repeat("wide ", 12)+"**", 20)

	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the text to wrap, got %d line(s)", len(lines))
	}
	for i, line := range lines {
		if strings.TrimSpace(stripANSI(line)) == "" {
			continue
		}
		if !hasAttr(line, "1") {
			t.Errorf("line %d lost the bold that was still open: %q", i, line)
		}
	}
}

func TestWrapRespectsWidth(t *testing.T) {
	got := md(t, strings.Repeat("word ", 40), 24)
	for _, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > 24 {
			t.Errorf("line is %d wide, want at most 24: %q", w, stripANSI(line))
		}
	}
}

func TestLinkKeepsBothHalves(t *testing.T) {
	plain := stripANSI(md(t, "see [the docs](https://example.com/x)", 60))
	if !strings.Contains(plain, "the docs") || !strings.Contains(plain, "https://example.com/x") {
		t.Errorf("link lost something: %q", plain)
	}
	if strings.Contains(plain, "](") {
		t.Errorf("link syntax leaked: %q", plain)
	}
}

func TestPlainTextIsUnchanged(t *testing.T) {
	plain := stripANSI(md(t, "just a sentence with no markup", 60))
	if plain != "just a sentence with no markup" {
		t.Errorf("text = %q", plain)
	}
}

func TestUserEntriesAreNotParsed(t *testing.T) {
	e := entry{kind: entryUser, text: "what does **kwargs mean"}
	if !strings.Contains(stripANSI(e.render(60)), "**kwargs") {
		t.Errorf("user text was reinterpreted: %q", stripANSI(e.render(60)))
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			if end := strings.IndexByte(s[i:], 'm'); end >= 0 {
				i += end + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
