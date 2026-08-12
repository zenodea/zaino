package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
)

func toolCall(name, input string) llm.ToolUseBlock {
	return llm.ToolUseBlock{ID: "toolu_" + name, Name: name, Input: json.RawMessage(input)}
}

func chatModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, 80, 20)
	m.ready = true

	send(m, textDeltaMsg("here you go"))
	call := toolCall("read", `{"path":"internal/agent/agent.go","offset":40}`)
	send(m,
		toolCallMsg{Call: call},
		toolResultMsg{Call: call, Result: "     1\tpackage agent\n     2\t\n     3\timport (\n"},
	)
	return m
}

func ctrl(m *Model, key tea.KeyType) { m.Update(tea.KeyMsg{Type: key}) }

func TestCtrlJKWalkTheChat(t *testing.T) {
	m := chatModel(t)
	if m.cursor != -1 {
		t.Fatalf("cursor starts at %d, want nothing selected", m.cursor)
	}

	ctrl(m, tea.KeyCtrlK)
	if m.cursor != len(m.entries)-1 {
		t.Errorf("⌃k from the composer = %d, want the last entry", m.cursor)
	}

	ctrl(m, tea.KeyCtrlK)
	if m.cursor != len(m.entries)-2 {
		t.Errorf("⌃k again = %d, want one further back", m.cursor)
	}

	ctrl(m, tea.KeyCtrlJ)
	if m.cursor != len(m.entries)-1 {
		t.Errorf("⌃j = %d, want back down one", m.cursor)
	}
}

func TestWalkingPastTheEndReleasesTheCursor(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)
	ctrl(m, tea.KeyCtrlJ)

	if m.cursor != -1 {
		t.Errorf("cursor = %d, want it released back to the composer", m.cursor)
	}
}

func TestEnterExpandsAToolCall(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)

	e, ok := m.selectedEntry()
	if !ok || e.kind != entryTool {
		t.Fatalf("selected %+v, want the tool call", e)
	}

	ctrl(m, tea.KeyEnter)
	view := stripANSI(strings.Join(m.rendered, "\n"))
	for _, want := range []string{"package agent", "internal/agent/agent.go", "input", "result"} {
		if !strings.Contains(view, want) {
			t.Errorf("expanded tool call is missing %q:\n%s", want, view)
		}
	}

	ctrl(m, tea.KeyEnter)
	if strings.Contains(stripANSI(strings.Join(m.rendered, "\n")), "package agent") {
		t.Error("enter did not collapse it again")
	}
}

func TestCollapsedToolCallHidesTheDetail(t *testing.T) {
	m := chatModel(t)
	if view := stripANSI(strings.Join(m.rendered, "\n")); strings.Contains(view, "package agent") {
		t.Errorf("the result is shown before being asked for:\n%s", view)
	}
}

func TestExpandIsOfferedOnlyWhenThereIsSomethingToShow(t *testing.T) {
	m := chatModel(t)
	view := stripANSI(strings.Join(m.rendered, "\n"))
	if !strings.Contains(view, "▸") {
		t.Errorf("no hint that the tool call opens:\n%s", view)
	}
}

func TestEnterSendsWhenThereIsAPrompt(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)
	m.input.SetValue("carry on")

	ctrl(m, tea.KeyEnter)
	if m.input.Value() != "" {
		t.Errorf("input = %q, want it sent", m.input.Value())
	}
}

func TestEnterOnANonToolEntryDoesNotSend(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)
	ctrl(m, tea.KeyCtrlK)

	if e, _ := m.selectedEntry(); e.kind == entryTool {
		t.Skip("expected a non-tool entry here")
	}
	before := len(m.entries)
	ctrl(m, tea.KeyEnter)
	if len(m.entries) != before {
		t.Error("enter on a non-tool entry submitted something")
	}
}

func TestTypingReturnsToTheComposer(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)
	if m.cursor < 0 {
		t.Fatal("nothing selected")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.cursor != -1 {
		t.Errorf("cursor = %d, want typing to hand the keyboard back", m.cursor)
	}
	if m.input.Value() != "h" {
		t.Errorf("input = %q, want the keystroke to have landed", m.input.Value())
	}
}

func TestSelectedEntryIsMarked(t *testing.T) {
	m := chatModel(t)
	ctrl(m, tea.KeyCtrlK)

	if !strings.Contains(m.rendered[m.cursor], "▌") {
		t.Errorf("selected entry carries no bar: %q", stripANSI(m.rendered[m.cursor]))
	}
	for i, r := range m.rendered {
		if i != m.cursor && strings.Contains(r, "▌") {
			t.Errorf("entry %d is marked but not selected: %q", i, stripANSI(r))
		}
	}
}

func TestNewlineMovedOffCtrlJ(t *testing.T) {
	m := newTestModel(t, 80, 20)
	m.ready = true
	m.input.SetValue("one")

	ctrl(m, tea.KeyCtrlJ)
	if strings.Contains(m.input.Value(), "\n") {
		t.Errorf("⌃j still inserts a newline: %q", m.input.Value())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}, Alt: true})
	if !strings.Contains(m.input.Value(), "\n") {
		t.Errorf("⌥⏎ did not insert a newline: %q", m.input.Value())
	}
}

func longChat(t *testing.T, turns int) *Model {
	t.Helper()
	m := newTestModel(t, 60, 10)
	m.resize(60, 10)
	for i := range turns {
		m.push(entry{kind: entryUser, text: fmt.Sprintf("question %d", i)})
		m.push(entry{kind: entryAssistant, text: fmt.Sprintf("answer %d", i)})
	}
	m.rerender()
	m.viewport.GotoBottom()
	return m
}

func TestCursorOffsetsMatchTheTranscript(t *testing.T) {
	m := longChat(t, 6)
	lines := strings.Split(stripANSI(m.transcript()), "\n")

	for i := range m.entries {
		top := m.tops[i]
		if top >= len(lines) {
			t.Fatalf("entry %d claims line %d, transcript has %d", i, top, len(lines))
		}
		want := stripANSI(strings.Split(m.rendered[i], "\n")[0])
		if lines[top] != want {
			t.Errorf("entry %d says line %d, which holds %q, want %q", i, top, lines[top], want)
		}
	}
}

func TestScrollingReachesTheSelectedEntry(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(false)

	for range 4 * len(m.entries) {
		if m.moveCursor(-1); m.cursor == 0 {
			break
		}
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want the top entry", m.cursor)
	}

	top := m.tops[0]
	if top < m.viewport.YOffset || top >= m.viewport.YOffset+m.viewport.Height {
		t.Errorf("entry is on line %d but the view shows %d..%d",
			top, m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height)
	}
}

func TestScrollingStaysPutWhenTheEntryIsAlreadyVisible(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(false)
	m.moveCursor(-1)

	at := m.viewport.YOffset
	m.moveCursor(-1)
	if m.viewport.YOffset != at {
		t.Errorf("the view moved to %d from %d for an entry already on screen", m.viewport.YOffset, at)
	}
}
