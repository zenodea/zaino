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

// The list grows up from where the composer normally sits, so a short list is
// next to the rule rather than stranded at the top of the screen.
func TestPickerGrowsUpFromTheBottom(t *testing.T) {
	m := pickerModel(t, 4)
	m.resize(80, 24)
	m.picker = picker{open: true, items: m.picker.items, cursor: 3}

	lines := strings.Split(stripANSI(m.pickerView()), "\n")
	if len(lines) != m.viewport.Height {
		t.Fatalf("picker drew %d lines, want the full %d", len(lines), m.viewport.Height)
	}
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("first line is %q, want blank padding above a short list", lines[0])
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last == "" {
		t.Error("the last line is blank; the list should end at the rule")
	}
}

func TestPickerFillsTheHeightWhenFull(t *testing.T) {
	m := pickerModel(t, 60)
	m.resize(80, 24)

	lines := strings.Split(stripANSI(m.pickerView()), "\n")
	if len(lines) > m.viewport.Height {
		t.Errorf("picker drew %d lines, more than the %d available", len(lines), m.viewport.Height)
	}
}

func TestResumeIsQuiet(t *testing.T) {
	m := pickerModel(t, 3)
	before := len(m.entries)
	m.resume(m.picker.items[0].ID)

	for _, e := range m.entries[min(before, len(m.entries)):] {
		if strings.Contains(e.text, "resumed") {
			t.Errorf("resuming announced itself: %q", e.text)
		}
	}
}

// The sessions list uses the same bar as the transcript, not a caret.
func TestThePickerDrawsTheCursorBar(t *testing.T) {
	m := pickerModel(t, 3)

	view := m.pickerView()
	if strings.Contains(view, "›") {
		t.Errorf("the picker still uses a caret:\n%s", view)
	}
	if !strings.Contains(view, "▌") {
		t.Errorf("the cursor bar is not drawn:\n%s", view)
	}
	if strings.Count(view, "▌") != 1 {
		t.Errorf("got %d bars, want exactly one:\n%s", strings.Count(view, "▌"), view)
	}
}

func TestThePickerBarFollowsTheCursor(t *testing.T) {
	m := pickerModel(t, 5)
	m.motion.on = false

	start := m.picker.barAt
	m.movePicker(1)
	if m.picker.barAt == start {
		t.Errorf("the bar did not move from %d", start)
	}
	if m.picker.barAt != m.picker.barTo {
		t.Errorf("without animation the bar should land at once: %d vs %d",
			m.picker.barAt, m.picker.barTo)
	}
}

func TestThePickerBarWalksWhenAnimated(t *testing.T) {
	m := pickerModel(t, 12)
	m.motion.on = true

	m.movePicker(6)
	if m.picker.barAt == m.picker.barTo {
		t.Skip("the window kept the cursor in place")
	}

	steps := 0
	for m.picker.barAt != m.picker.barTo && steps < 100 {
		m.advancePickerBar()
		steps++
	}
	if steps == 0 {
		t.Error("the bar arrived without a single step")
	}
	if m.picker.barAt != m.picker.barTo {
		t.Errorf("the bar never arrived: %d vs %d", m.picker.barAt, m.picker.barTo)
	}
	if len(m.picker.trail) == 0 {
		t.Error("the bar left no trail behind it")
	}
}

func TestThePickerTrailFades(t *testing.T) {
	m := pickerModel(t, 6)
	m.leavePickerTrail(2)

	for range trailLife() + 2 {
		m.advancePickerBar()
	}
	if len(m.picker.trail) != 0 {
		t.Errorf("the trail outlived its life: %v", m.picker.trail)
	}
}
