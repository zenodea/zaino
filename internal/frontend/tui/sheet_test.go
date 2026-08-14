package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

// Commands that answer rather than ask get a screen too, not a wall of text
// in the transcript.
func TestAnsweringCommandsOpenASheet(t *testing.T) {
	for _, line := range []string{"/help", "/tools", "/usage", "/system"} {
		t.Run(line, func(t *testing.T) {
			m := chooserModel(t)
			w, _ := tool.NewWorkspace(t.TempDir())
			m.agent.Tools = tool.All(w)

			before := len(m.entries)
			m.runCommand(line)

			if !m.sheet.open {
				t.Fatalf("%s did not open a sheet", line)
			}
			if m.sheet.title == "" {
				t.Errorf("%s opened an untitled sheet", line)
			}
			if len(m.entries) != before {
				t.Errorf("%s also wrote %d entries to the transcript", line, len(m.entries)-before)
			}
		})
	}
}

func TestASheetTakesTheScreen(t *testing.T) {
	m := chooserModel(t)
	tall := m.viewport.Height
	m.runCommand("/help")

	if m.viewport.Height != tall {
		t.Errorf("the sheet took %d rows from the transcript", tall-m.viewport.Height)
	}

	view := stripANSI(m.View())
	if strings.Contains(view, "Ask something") {
		t.Errorf("the composer is still on screen behind the sheet:\n%s", view)
	}
	if !strings.Contains(view, "esc") {
		t.Errorf("the sheet does not say how to leave:\n%s", view)
	}
}

func TestASheetIsAlwaysTheSameHeight(t *testing.T) {
	m := chooserModel(t)

	heights := map[int]bool{}
	for _, line := range []string{"/help", "/system", "/usage"} {
		m.runCommand(line)
		heights[len(strings.Split(m.View(), "\n"))] = true
		m.closeSheet()
	}
	if len(heights) != 1 {
		t.Errorf("sheets came out at heights %v; the footer should not move", heights)
	}
}

func TestASheetScrollsAndCloses(t *testing.T) {
	m := chooserModel(t)
	m.show("long", longLines(200))

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.sheet.offset != 1 {
		t.Errorf("offset = %d after one j", m.sheet.offset)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if m.sheet.offset != 200-m.sheetRows() {
		t.Errorf("offset = %d after G, want the end", m.sheet.offset)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.sheet.offset != 0 {
		t.Errorf("offset = %d after g", m.sheet.offset)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.sheet.open {
		t.Error("esc did not close the sheet")
	}
}

func TestASheetWillNotScrollPastItsEnds(t *testing.T) {
	m := chooserModel(t)
	m.show("short", []string{"one", "two"})

	for range 20 {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.sheet.offset != 0 {
		t.Errorf("offset = %d, want a sheet shorter than the screen to stay put", m.sheet.offset)
	}
}

func longLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "line"
	}
	return out
}

// The marks say what each tool will actually do under the mode in force, so
// they are asked of the policy like the permission board is.
func TestToolsSheetFollowsTheMode(t *testing.T) {
	tests := []struct {
		mode        permission.Mode
		read, write int
	}{
		{permission.Manual, stateAllow, stateAsk},
		{permission.AcceptEdits, stateAllow, stateAllow},
		{permission.Plan, stateAllow, stateRefuse},
		{permission.Bypass, stateAllow, stateAllow},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			m := chooserModel(t)
			m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(tt.mode)}

			if got := stateFor(m, permission.Read); got != tt.read {
				t.Errorf("read = %d, want %d", got, tt.read)
			}
			if got := stateFor(m, permission.Write); got != tt.write {
				t.Errorf("write = %d, want %d", got, tt.write)
			}
		})
	}
}

func TestEveryToolDeclaresWhatItAsksFor(t *testing.T) {
	w, err := tool.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, tl := range tool.All(w) {
		if _, ok := tl.(tool.Asks); !ok {
			t.Errorf("%s does not say what it asks under, so /tools has to guess",
				tl.Definition().Name)
		}
	}
}

func TestUsageShowsHowFullTheWindowIs(t *testing.T) {
	m := chooserModel(t)
	m.resize(90, 26)
	m.agent.Compaction = &agent.Compaction{Window: 200_000}
	send(m, turnMsg{Usage: llm.Usage{InputTokens: 34_900, OutputTokens: 2_200}})

	m.runCommand("/usage")
	text := sheetText(t, m)

	for _, want := range []string{"context", "input", "output", "200.0k"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage is missing %q:\n%s", want, text)
		}
	}
}

func TestUsageLeavesOutTheWindowWhenNothingCompacts(t *testing.T) {
	m := chooserModel(t)
	m.resize(90, 26)
	m.agent.Compaction = nil

	m.runCommand("/usage")
	if strings.Contains(sheetText(t, m), "context") {
		t.Error("usage claims a context window with compaction switched off")
	}
}
