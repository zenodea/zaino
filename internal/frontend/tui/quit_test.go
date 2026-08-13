package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/permission"
)

func quitModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, 80, 20)
	m.resize(80, 20)
	m.push(entry{kind: entryUser, text: "something said"})
	m.rerender()
	return m
}

func TestOneCtrlCDoesNotQuit(t *testing.T) {
	m := quitModel(t)

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.quitting {
		t.Error("a single ⌃c quit")
	}
	if !m.quitArmed {
		t.Error("a single ⌃c did not arm the quit")
	}
	if got := stripANSI(m.footer()); !strings.Contains(got, "again to quit") {
		t.Errorf("footer = %q, want it to say what the second press does", got)
	}
}

func TestTwoCtrlCsQuit(t *testing.T) {
	m := quitModel(t)

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitting {
		t.Error("⌃c twice did not quit")
	}
}

func TestAnyKeyStandsTheQuitDown(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("h")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEscape},
		{Type: tea.KeyCtrlK},
	} {
		m := quitModel(t)
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m.Update(key)

		if m.quitArmed {
			t.Errorf("%v left the quit armed", key)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if m.quitting {
			t.Errorf("%v then ⌃c quit; the first press should have lapsed", key)
		}
	}
}

func TestCtrlCStopsTheTurnBeforeItArms(t *testing.T) {
	m := quitModel(t)
	cancelled := false
	m.streaming, m.cancel = true, func() { cancelled = true }

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !cancelled {
		t.Error("⌃c did not stop the running turn")
	}
	if m.quitArmed || m.quitting {
		t.Error("stopping a turn armed the quit as well")
	}
}

func TestCtrlDNoLongerQuits(t *testing.T) {
	m := quitModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if m.quitting {
		t.Error("⌃d quit")
	}
}

func TestCtrlLNoLongerClears(t *testing.T) {
	m := quitModel(t)
	before := len(m.entries)

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if len(m.entries) != before {
		t.Errorf("⌃l changed the transcript: %d entries, was %d", len(m.entries), before)
	}
}

func TestQuitCommandStillWorks(t *testing.T) {
	for _, name := range []string{"quit", "exit", "q"} {
		if _, ok := lookupCommand(name); !ok {
			t.Errorf("/%s is not a command", name)
		}
	}
}

func TestClearCommandStillWorks(t *testing.T) {
	m := quitModel(t)
	m.runCommand("/clear")

	if len(m.entries) == 0 {
		t.Fatal("/clear left nothing at all, not even the notice")
	}
	if len(m.messages) != 0 {
		t.Errorf("/clear left %d messages", len(m.messages))
	}
}

func TestEscapeStopsARunningTurn(t *testing.T) {
	m := quitModel(t)
	cancelled := false
	m.streaming, m.cancel = true, func() { cancelled = true }

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !cancelled {
		t.Error("esc did not stop the running turn")
	}
	if m.quitting {
		t.Error("esc quit")
	}
}

func TestEscapeWithNothingRunningIsHarmless(t *testing.T) {
	m := quitModel(t)
	m.UseVim(false)
	m.input.SetValue("half a thought")
	before, height := len(m.entries), m.viewport.Height

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.input.Value() != "half a thought" {
		t.Errorf("esc changed the composer to %q", m.input.Value())
	}
	if len(m.entries) != before {
		t.Errorf("esc changed the transcript: %d entries, was %d", len(m.entries), before)
	}
	if m.viewport.Height != height {
		t.Errorf("esc resized the viewport: %d, was %d", m.viewport.Height, height)
	}
	if m.quitting || m.quitArmed {
		t.Error("esc armed or performed a quit")
	}
}

func TestEscapeStillEntersNormalMode(t *testing.T) {
	m := quitModel(t)
	m.UseVim(true)

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.inNormalMode() {
		t.Error("esc did not enter normal mode")
	}
}

func TestEscapeLeavesInsertBeforeStoppingTheTurn(t *testing.T) {
	m := quitModel(t)
	m.UseVim(true)
	cancelled := false
	m.streaming, m.cancel = true, func() { cancelled = true }

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelled {
		t.Error("esc stopped the turn while still in insert mode")
	}
	if !m.inNormalMode() {
		t.Fatal("esc did not enter normal mode")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !cancelled {
		t.Error("esc in normal mode did not stop the turn")
	}
}

func TestEscapeLeavesVisualBeforeStoppingTheTurn(t *testing.T) {
	m := quitModel(t)
	m.UseVim(true)
	m.input.SetValue("some text")
	cancelled := false
	m.streaming, m.cancel = true, func() { cancelled = true }

	keys(m, "⎋v")
	if !m.inVisualMode() {
		t.Fatal("v did not enter visual mode")
	}

	keys(m, "⎋")
	if cancelled {
		t.Error("esc stopped the turn instead of leaving visual mode")
	}
	if m.inVisualMode() {
		t.Fatal("esc did not leave visual mode")
	}

	keys(m, "⎋")
	if !cancelled {
		t.Error("esc in normal mode did not stop the turn")
	}
}

func TestEscapeClearsAPendingOperatorBeforeStoppingTheTurn(t *testing.T) {
	m := quitModel(t)
	m.UseVim(true)
	m.input.SetValue("some text")
	cancelled := false
	m.streaming, m.cancel = true, func() { cancelled = true }

	keys(m, "⎋2d")
	keys(m, "⎋")
	if cancelled {
		t.Error("esc stopped the turn instead of dropping the half-typed 2d")
	}

	keys(m, "⎋")
	if !cancelled {
		t.Error("esc did not stop the turn once there was nothing else to drop")
	}
}

func TestViewportHeightTracksTheMenu(t *testing.T) {
	m := quitModel(t)
	tall := m.viewport.Height

	typeLine(m, "/m")
	if m.viewport.Height >= tall {
		t.Fatalf("viewport did not shrink for the panel: %d then %d", tall, m.viewport.Height)
	}

	m.complete()
	if m.menu.open {
		t.Skip("completion left the panel open")
	}
	if m.viewport.Height != tall {
		t.Errorf("viewport is %d after the panel closed, want %d back", m.viewport.Height, tall)
	}
}

// The footer already carries the mode; saying it again in the transcript is
// noise that outlives the moment.
func TestCyclingTheModeIsQuiet(t *testing.T) {
	m := quitModel(t)
	m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Manual)}
	before := len(m.entries)

	for range 4 {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	}

	if len(m.entries) != before {
		t.Errorf("cycling the mode added %d transcript entries", len(m.entries)-before)
	}
	if got := stripANSI(m.footer()); !strings.Contains(got, "manual") {
		t.Errorf("footer = %q, want the mode still shown there", got)
	}
}
