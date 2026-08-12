package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/permission"
)

func vimModel(t *testing.T, text string) *Model {
	t.Helper()
	m := newTestModel(t, 80, 24)
	m.UseVim(true)
	m.input.SetValue(text)
	m.input.CursorEnd()
	return m
}

func keys(m *Model, sequence string) {
	for _, r := range sequence {
		switch r {
		case '⎋':
			m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		case '↑':
			m.Update(tea.KeyMsg{Type: tea.KeyUp})
		default:
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
}

func TestVimStartsInInsert(t *testing.T) {
	m := vimModel(t, "")
	if m.inNormalMode() {
		t.Fatal("vim started in normal mode")
	}

	keys(m, "hello")
	if got := m.input.Value(); got != "hello" {
		t.Errorf("typing in insert mode = %q, want %q", got, "hello")
	}
}

func TestEscEntersNormalMode(t *testing.T) {
	m := vimModel(t, "hello")
	keys(m, "⎋")
	if !m.inNormalMode() {
		t.Fatal("esc did not enter normal mode")
	}

	keys(m, "iii")
	if got := m.input.Value(); got == "helloiii" {
		t.Errorf("normal-mode keys were typed into the buffer: %q", got)
	}
}

func TestVimOffLeavesEscAlone(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.UseVim(false)
	m.input.SetValue("hello")

	keys(m, "⎋x")
	if got := m.input.Value(); got != "hellox" {
		t.Errorf("value = %q, want esc ignored and x typed", got)
	}
}

func TestNormalModeEdits(t *testing.T) {
	tests := []struct {
		name string
		text string
		keys string
		want string
	}{
		{"x deletes under cursor", "abc", "⎋0x", "bc"},
		{"count with x", "abcdef", "⎋03x", "def"},
		{"dd clears the line", "one line", "⎋dd", ""},
		{"D to end of line", "keep this away", "⎋0wD", "keep "},
		{"dw deletes a word", "alpha beta", "⎋0dw", "beta"},
		{"2dw deletes two", "alpha beta gamma", "⎋02dw", "gamma"},
		{"db deletes back", "alpha beta", "⎋$db", "alpha a"},
		{"d$ to end", "alpha beta", "⎋0wd$", "alpha "},
		{"X deletes before", "abc", "⎋$X", "ac"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := vimModel(t, tt.text)
			keys(m, tt.keys)
			if got := m.input.Value(); got != tt.want {
				t.Errorf("after %q: %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

func TestChangeEntersInsert(t *testing.T) {
	m := vimModel(t, "alpha beta")
	keys(m, "⎋0cw")

	if m.inNormalMode() {
		t.Error("cw left the editor in normal mode")
	}
	keys(m, "omega ")
	if got := m.input.Value(); got != "omega beta" {
		t.Errorf("value = %q, want %q", got, "omega beta")
	}
}

func TestInsertEntryPoints(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"i inserts before", "⎋0iX", "Xabc"},
		{"a inserts after", "⎋0aX", "aXbc"},
		{"A appends to the line", "⎋0AX", "abcX"},
		{"I goes to the first character", "⎋$IX", "Xabc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := vimModel(t, "abc")
			keys(m, tt.keys)
			if got := m.input.Value(); got != tt.want {
				t.Errorf("after %q: %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

func TestUndo(t *testing.T) {
	m := vimModel(t, "keep this")
	keys(m, "⎋dd")
	if m.input.Value() != "" {
		t.Fatalf("dd left %q", m.input.Value())
	}

	keys(m, "u")
	if got := m.input.Value(); got != "keep this" {
		t.Errorf("after u: %q, want the line back", got)
	}
}

func TestDeleteThenPaste(t *testing.T) {
	m := vimModel(t, "alpha beta")
	keys(m, "⎋0dw")
	if got := m.input.Value(); got != "beta" {
		t.Fatalf("dw left %q", got)
	}

	keys(m, "$p")
	if got := m.input.Value(); !strings.Contains(got, "alpha ") {
		t.Errorf("after p: %q, want the deleted text back", got)
	}
}

func TestEnterSendsFromNormalMode(t *testing.T) {
	m := vimModel(t, "ask something")
	keys(m, "⎋")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.input.Value() != "" {
		t.Errorf("input = %q, want it sent and cleared", m.input.Value())
	}
	if len(m.entries) == 0 {
		t.Error("nothing was submitted")
	}
}

func TestNormalModeKeepsAppKeys(t *testing.T) {
	m := vimModel(t, "")
	m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Manual)}
	keys(m, "⎋")

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.agent.Gate.Mode() == permission.Manual {
		t.Error("⇧⇥ did not cycle the permission mode from normal mode")
	}
}

func TestModeIndicatorIsOnScreen(t *testing.T) {
	m := vimModel(t, "")
	m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.AcceptEdits)}

	footer := stripANSI(m.footer())
	if !strings.Contains(footer, "accept edits") {
		t.Errorf("footer does not show the permission mode:\n%s", footer)
	}
	if !strings.Contains(footer, "INSERT") {
		t.Errorf("footer does not show the vim mode:\n%s", footer)
	}

	keys(m, "⎋")
	if footer := stripANSI(m.footer()); !strings.Contains(footer, "NORMAL") {
		t.Errorf("footer did not follow the mode change:\n%s", footer)
	}
}

func TestEveryPermissionModeHasAChip(t *testing.T) {
	m := vimModel(t, "")
	for _, mode := range []permission.Mode{
		permission.Manual, permission.AcceptEdits, permission.Plan, permission.Bypass,
	} {
		m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(mode)}
		if chip := stripANSI(m.status()); strings.TrimSpace(chip) == "" {
			t.Errorf("%q has no indicator", mode)
		}
	}
}

func TestFooterNeverOverflows(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 140} {
		m := vimModel(t, "")
		m.resize(width, 24)
		m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.AcceptEdits)}
		m.sessionUsage.InputTokens = 128000
		m.sessionUsage.OutputTokens = 64000

		for _, normal := range []bool{false, true} {
			if normal {
				keys(m, "⎋")
			}
			if got := lipglossWidth(m.footer()); got > m.contentWidth() {
				t.Errorf("at %d columns the footer is %d wide, room is %d: %q",
					width, got, m.contentWidth(), stripANSI(m.footer()))
			}
			if !strings.Contains(stripANSI(m.footer()), "accept edits") {
				t.Errorf("at %d columns the permission mode was dropped: %q",
					width, stripANSI(m.footer()))
			}
		}
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }
