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

	lines := strings.Split(stripANSI(m.transcript()), "\n")
	marked := 0
	for i, line := range lines {
		if !strings.Contains(line, "▌") {
			continue
		}
		marked++
		if i != m.tops[m.cursor] {
			t.Errorf("line %d is marked but the selected entry starts at %d: %q",
				i, m.tops[m.cursor], line)
		}
	}
	if marked != 1 {
		t.Errorf("%d lines carry a bar, want only the top line of the selected entry", marked)
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
		// The bar lives in the transcript rather than in the entry, so the
		// comparison is of what comes after the gutter.
		want := afterGutter(stripANSI(strings.Split(m.rendered[i], "\n")[0]))
		if got := afterGutter(lines[top]); got != want {
			t.Errorf("entry %d says line %d, which holds %q, want %q", i, top, got, want)
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

func afterGutter(line string) string {
	if runes := []rune(line); len(runes) > gutterWidth {
		return string(runes[gutterWidth:])
	}
	return ""
}

// A three-line error with a bar down every line is a wall, so it is shown by
// its first line until it is asked for.
func TestLongErrorsFold(t *testing.T) {
	m := chooserModel(t)
	m.ready = true
	m.push(entry{kind: entryError, text: "gemini: 429 RESOURCE_EXHAUSTED: quota exceeded\nsee the rate limit docs\n* limit 20"})
	m.rerender()

	view := stripANSI(m.transcript())
	if strings.Contains(view, "limit 20") {
		t.Errorf("the whole error is on screen before being asked for:\n%s", view)
	}
	if !strings.Contains(view, "▸") {
		t.Errorf("nothing says the error opens:\n%s", view)
	}

	m.moveCursor(-1)
	m.toggleSelected()
	if opened := stripANSI(m.transcript()); !strings.Contains(opened, "limit 20") {
		t.Errorf("enter did not open the error:\n%s", opened)
	}
}

func TestAOneLineErrorDoesNotFold(t *testing.T) {
	m := chooserModel(t)
	m.ready = true
	m.push(entry{kind: entryError, text: "no such tool"})
	m.rerender()

	if view := stripANSI(m.transcript()); !strings.Contains(view, "✗") {
		t.Errorf("a short error should keep its own marker:\n%s", view)
	}
}

// Whatever the entry's height, only the line it starts on is marked.
func TestOnlyTheTopLineIsMarked(t *testing.T) {
	m := chooserModel(t)
	m.resize(50, 24)
	m.ready = true
	m.push(entry{kind: entryAssistant, text: strings.Repeat("a long answer that wraps several times ", 8)})
	m.rerender()

	m.UseAnimation(false)
	m.moveCursor(-1)

	lines := strings.Split(stripANSI(m.transcript()), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected the entry to wrap, got %d lines", len(lines))
	}
	for i, line := range lines[1:] {
		if strings.Contains(line, "▌") {
			t.Errorf("line %d of the entry is marked too: %q", i+1, line)
		}
	}
}

// The bar covers a long gap in about the same time as a short one.
func TestTravelTakesTheSameTimeHoweverFar(t *testing.T) {
	frames := func(entries int) int {
		m := longChat(t, entries)
		m.UseAnimation(true)
		m.moveCursor(-1)
		m.moveCursor(-2 * entries)

		n := 0
		for range 400 {
			if m.motion.barAt == m.motion.barTo {
				break
			}
			m.Update(frameMsg{})
			n++
		}
		return n
	}

	near, far := frames(4), frames(30)
	if far > near*3 {
		t.Errorf("a long move took %d frames against %d for a short one", far, near)
	}
}
