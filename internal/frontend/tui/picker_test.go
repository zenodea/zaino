package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/store/session"
)

func pickerModel(t *testing.T, n int) *Model {
	t.Helper()
	m := newTestModel(t, 80, 24)
	m.ready = true

	items := make([]session.Summary, n)
	for i := range items {
		items[i] = session.Summary{
			Meta:     session.Meta{ID: string(rune('a' + i))},
			Updated:  time.Now(),
			Messages: i,
			Preview:  "session " + string(rune('a'+i)),
		}
	}
	m.picker = picker{open: true, items: items}
	return m
}

func TestPickerVimKeys(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want int
	}{
		{"j moves down", "jj", 2},
		{"k moves back", "jjjk", 2},
		{"G goes to the end", "G", 9},
		{"g goes to the top", "jjjg", 0},
		{"j stops at the end", "jjjjjjjjjjjjjj", 9},
		{"k stops at the top", "kkk", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pickerModel(t, 10)
			for _, r := range tt.keys {
				m.handlePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			if m.picker.cursor != tt.want {
				t.Errorf("after %q cursor = %d, want %d", tt.keys, m.picker.cursor, tt.want)
			}
		})
	}
}

func TestPickerArrowsStillWork(t *testing.T) {
	m := pickerModel(t, 10)
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyDown})
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.picker.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.picker.cursor)
	}
}

func TestPickerCloses(t *testing.T) {
	for _, key := range []string{"q", "h"} {
		m := pickerModel(t, 3)
		m.handlePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if m.picker.open {
			t.Errorf("%q did not close the picker", key)
		}
	}

	m := pickerModel(t, 3)
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.picker.open {
		t.Error("esc did not close the picker")
	}
}

func TestPickerHidesTheComposer(t *testing.T) {
	m := pickerModel(t, 3)
	view := stripANSI(m.View())

	if strings.Contains(view, "Ask something") {
		t.Errorf("the composer is still on screen:\n%s", view)
	}
	if !strings.Contains(view, "session a") {
		t.Errorf("the session list is missing:\n%s", view)
	}
	if !strings.Contains(view, "resume") {
		t.Errorf("the picker keys are not shown:\n%s", view)
	}
}

func TestComposerReturnsWhenThePickerCloses(t *testing.T) {
	m := pickerModel(t, 3)
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEscape})

	if view := stripANSI(m.View()); !strings.Contains(view, "Ask something") {
		t.Errorf("the composer did not come back:\n%s", view)
	}
}

func TestPickerCountsItself(t *testing.T) {
	m := pickerModel(t, 10)
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	if view := stripANSI(m.pickerView()); !strings.Contains(view, "2 of 10") {
		t.Errorf("picker does not say where you are:\n%s", view)
	}
}

func TestWheelScrollsTheTranscriptNotTheComposer(t *testing.T) {
	m := newTestModel(t, 80, 10)
	m.ready = true
	for i := range 40 {
		m.push(entry{kind: entryAssistant, text: fmt.Sprintf("line %d", i)})
	}
	m.viewport.GotoBottom()
	at := m.viewport.YOffset

	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.viewport.YOffset >= at {
		t.Errorf("wheel up left the transcript at %d (was %d)", m.viewport.YOffset, at)
	}
	if m.input.Value() != "" {
		t.Errorf("the wheel typed into the composer: %q", m.input.Value())
	}
}

func TestWheelMovesThePicker(t *testing.T) {
	m := pickerModel(t, 10)
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})

	if m.picker.cursor != 2 {
		t.Errorf("cursor = %d, want the wheel to have moved it to 2", m.picker.cursor)
	}
}
