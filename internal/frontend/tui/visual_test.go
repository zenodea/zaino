package tui

import (
	"strings"
	"testing"
)

func TestVisualSelectsAndDeletes(t *testing.T) {
	tests := []struct {
		name string
		text string
		keys string
		want string
	}{
		{"vd deletes one character", "abc", "⎋0vd", "bc"},
		{"v with a motion", "alpha beta", "⎋0ved", " beta"},
		{"v$ to the end of the line", "alpha beta", "⎋0wv$d", "alpha "},
		{"vhd back over one", "abc", "⎋$vhd", "a"},
		{"x is d in visual mode", "abc", "⎋0vx", "bc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := vimModel(t, tt.text)
			keys(m, tt.keys)
			if got := m.input.Value(); got != tt.want {
				t.Errorf("after %q: %q, want %q", tt.keys, got, tt.want)
			}
			if m.inVisualMode() {
				t.Error("still in visual mode after an operator")
			}
		})
	}
}

func TestVisualChangeEntersInsert(t *testing.T) {
	m := vimModel(t, "alpha beta")
	keys(m, "⎋0vec")

	if m.inNormalMode() {
		t.Error("c left the editor in normal mode")
	}
	keys(m, "omega")
	if got := m.input.Value(); got != "omega beta" {
		t.Errorf("value = %q, want %q", got, "omega beta")
	}
}

func TestVisualYankThenPaste(t *testing.T) {
	m := vimModel(t, "alpha beta")
	keys(m, "⎋0vey")

	if m.inVisualMode() {
		t.Error("y did not leave visual mode")
	}
	if got := m.input.Value(); got != "alpha beta" {
		t.Errorf("y changed the buffer: %q", got)
	}

	keys(m, "$p")
	if got := m.input.Value(); !strings.Contains(got, "alpha") || got == "alpha beta" {
		t.Errorf("after p: %q, want the yanked text pasted", got)
	}
}

func TestVisualLineTakesTheWholeLine(t *testing.T) {
	m := vimModel(t, "keep\nthis line\nand this")
	keys(m, "⎋kVd")

	if got := m.input.Value(); strings.Contains(got, "this line") {
		t.Errorf("V-line delete left %q", got)
	}
}

func TestEscLeavesVisualWithoutChanging(t *testing.T) {
	m := vimModel(t, "abc")
	keys(m, "⎋0ve⎋")

	if m.inVisualMode() {
		t.Error("esc did not leave visual mode")
	}
	if !m.inNormalMode() {
		t.Error("esc left normal mode as well")
	}
	if got := m.input.Value(); got != "abc" {
		t.Errorf("value = %q, want it untouched", got)
	}
}

func TestVisualTogglesOff(t *testing.T) {
	m := vimModel(t, "abc")
	keys(m, "⎋v")
	if !m.inVisualMode() {
		t.Fatal("v did not enter visual mode")
	}
	keys(m, "v")
	if m.inVisualMode() {
		t.Error("v again did not leave visual mode")
	}
}

func TestSelectionIsVisible(t *testing.T) {
	m := vimModel(t, "alpha beta")
	m.resize(80, 24)
	keys(m, "⎋0ve")

	view := m.visualView()
	if stripANSI(view) != "alpha beta" {
		t.Errorf("visual view = %q, want the buffer unchanged", stripANSI(view))
	}

	before, selected := strings.Index(view, "alpha"), false
	for _, style := range strings.Split(view, "\x1b[") {
		if strings.Contains(style, "alpha") && strings.Contains(style, "48;") {
			selected = true
		}
	}
	if !selected {
		t.Errorf("the selected run has no highlight (at %d):\n%q", before, view)
	}
}

func TestVisualModeShowsInTheFooter(t *testing.T) {
	m := vimModel(t, "abc")
	m.resize(90, 24)
	keys(m, "⎋v")

	if got := stripANSI(m.footer()); !strings.Contains(got, "VISUAL") {
		t.Errorf("footer = %q, want it to say VISUAL", got)
	}

	keys(m, "V")
	if got := stripANSI(m.footer()); !strings.Contains(got, "V-LINE") {
		t.Errorf("footer = %q, want it to say V-LINE", got)
	}
}

func TestInsertClearsVisual(t *testing.T) {
	m := vimModel(t, "abc")
	keys(m, "⎋vi")
	if m.inVisualMode() {
		t.Error("i left visual mode set")
	}
}
