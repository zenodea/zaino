package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type listingProvider struct{ stubProvider }

func (listingProvider) Models() []string {
	return []string{"claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5"}
}

func chooserModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, 90, 30)
	m.resize(90, 30)
	m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Manual)}
	return m
}

// A command with no argument asks rather than describing the options.
func TestBareCommandsOpenAChooser(t *testing.T) {
	for _, line := range []string{"/effort", "/thinking", "/permission", "/vim", "/provider"} {
		t.Run(line, func(t *testing.T) {
			m := chooserModel(t)
			m.runCommand(line)

			if !m.chooser.open {
				t.Fatalf("%s did not open a chooser", line)
			}
			if len(m.chooser.options) == 0 {
				t.Errorf("%s opened an empty chooser", line)
			}
			if m.chooser.title == "" {
				t.Errorf("%s opened an untitled chooser", line)
			}
		})
	}
}

func TestChooserStartsOnWhatIsInEffect(t *testing.T) {
	m := chooserModel(t)
	m.agent.Effort = llm.EffortXHigh

	m.runCommand("/effort")
	if got := m.chooser.options[m.chooser.cursor].value; got != llm.EffortXHigh {
		t.Errorf("cursor started on %q, want the effort in effect", got)
	}
	if m.chooser.current != llm.EffortXHigh {
		t.Errorf("current = %q", m.chooser.current)
	}
}

func TestChoosingAppliesIt(t *testing.T) {
	m := chooserModel(t)
	m.agent.Effort = llm.EffortXHigh

	m.runCommand("/effort")
	for m.chooser.options[m.chooser.cursor].value != llm.EffortLow {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.chooser.open {
		t.Error("the chooser stayed open after a pick")
	}
	if m.agent.Effort != llm.EffortLow {
		t.Errorf("effort = %q, want it applied", m.agent.Effort)
	}
}

func TestEscapeCancelsWithoutChanging(t *testing.T) {
	m := chooserModel(t)
	m.agent.Effort = llm.EffortXHigh

	m.runCommand("/effort")
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.chooser.open {
		t.Error("esc did not close the chooser")
	}
	if m.agent.Effort != llm.EffortXHigh {
		t.Errorf("effort = %q, want it untouched", m.agent.Effort)
	}
}

func TestChooserTakesVimKeys(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/permission")

	at := m.chooser.cursor
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.chooser.cursor == at {
		t.Error("j did not move the cursor")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.chooser.cursor != at {
		t.Error("k did not move back")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if m.chooser.cursor != len(m.chooser.options)-1 {
		t.Error("G did not go to the end")
	}
}

func TestChooserWrapsAround(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/thinking")

	m.chooser.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.chooser.cursor != len(m.chooser.options)-1 {
		t.Errorf("cursor = %d, want it wrapped to the end", m.chooser.cursor)
	}
}

func TestChooserShowsWhatIsInEffect(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/permission")

	view := stripANSI(m.boardView())
	if !strings.Contains(view, "permission") {
		t.Errorf("no title:\n%s", view)
	}
	if !strings.Contains(view, "ask before writing") {
		t.Errorf("options carry no explanation:\n%s", view)
	}
	if !strings.Contains(view, "·") {
		t.Errorf("nothing marks what is currently in effect:\n%s", view)
	}
}

func TestModelChooserListsWhatTheProviderKnows(t *testing.T) {
	m := chooserModel(t)
	m.agent.Provider = listingProvider{}

	m.runCommand("/model")
	if !m.chooser.open {
		t.Fatal("no chooser for a provider that lists its models")
	}
	if len(m.chooser.options) != 4 {
		t.Errorf("got %d options, want the default plus three models", len(m.chooser.options))
	}
}

func TestAnArgumentSkipsTheChooser(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/effort low")

	if m.chooser.open {
		t.Error("an explicit argument still opened a chooser")
	}
	if m.agent.Effort != llm.EffortLow {
		t.Errorf("effort = %q", m.agent.Effort)
	}
}

func TestChooserOwnsTheKeyboard(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/effort")

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if got := m.input.Value(); got != "" {
		t.Errorf("input = %q, want the keys to have gone to the chooser", got)
	}
}

// Something else animating must not drag the transcript with it.
// Something else animating must not drag the transcript with it.
func TestAnAnimatingChooserDoesNotScrollTheTranscript(t *testing.T) {
	m := chooserModel(t)
	m.UseAnimation(true)
	for i := range 60 {
		m.push(entry{kind: entryUser, text: fmt.Sprintf("line %d", i)})
	}
	m.rerender()
	m.viewport.GotoBottom()
	at := m.viewport.YOffset

	m.runCommand("/effort")
	m.moveChooser(3)
	for range 60 {
		if _, cmd := m.Update(frameMsg{}); cmd == nil {
			break
		}
	}

	if m.viewport.YOffset != at {
		t.Errorf("the transcript moved from %d to %d while a chooser animated", at, m.viewport.YOffset)
	}
}

// Everything that asks gets a screen; a scale is the one shape that reads
// better lying across it than down it.
func TestEachCommandGetsTheWeightItEarns(t *testing.T) {
	tests := []struct {
		line string
		want layout
	}{
		{"/thinking", layoutBoard},
		{"/vim", layoutBoard},
		{"/provider", layoutBoard},
		{"/model", layoutBoard},
		{"/permission", layoutBoard},
		{"/effort", layoutScale},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			m := chooserModel(t)
			m.agent.Provider = listingProvider{}
			m.runCommand(tt.line)

			if m.chooser.layout != tt.want {
				t.Errorf("%s uses layout %d, want %d", tt.line, m.chooser.layout, tt.want)
			}
		})
	}
}

func TestTheScaleTakesBothAxes(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyRight, tea.KeyDown} {
		m := chooserModel(t)
		m.runCommand("/effort")
		at := m.chooser.cursor

		m.Update(tea.KeyMsg{Type: key})
		if m.chooser.cursor == at {
			t.Errorf("%v did nothing on a scale; no arrow should be dead", key)
		}
	}
}

func TestTheScaleShowsEveryStop(t *testing.T) {
	m := chooserModel(t)
	m.resize(70, 24)
	m.runCommand("/effort")

	view := stripANSI(m.scaleView())
	for _, stop := range append([]string{"default"}, efforts...) {
		if !strings.Contains(view, stop) {
			t.Errorf("the scale is missing %q, so it misstates its own range:\n%s", stop, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if lipglossWidth(line) > m.contentWidth() {
			t.Errorf("scale line is %d wide, room is %d", lipglossWidth(line), m.contentWidth())
		}
	}
}

// The grid is asked of a real policy, so it cannot claim something the gate
// would not actually do.
func TestTheBoardGridMatchesThePolicy(t *testing.T) {
	tests := []struct {
		mode  permission.Mode
		write int
		fetch int
	}{
		{permission.Manual, stateAsk, stateAsk},
		{permission.AcceptEdits, stateAllow, stateAsk},
		{permission.Plan, stateRefuse, stateAllow},
		{permission.Bypass, stateAllow, stateAllow},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			cells := capabilities(tt.mode)
			if len(cells) != 4 {
				t.Fatalf("got %d cells", len(cells))
			}
			if cells[0].state != stateAllow {
				t.Errorf("read = %d, want it always allowed", cells[0].state)
			}
			if cells[1].state != tt.write {
				t.Errorf("write = %d, want %d", cells[1].state, tt.write)
			}
			if cells[3].state != tt.fetch {
				t.Errorf("fetch = %d, want %d", cells[3].state, tt.fetch)
			}
		})
	}
}

func TestTheBoardTakesTheScreen(t *testing.T) {
	m := chooserModel(t)
	tall := m.viewport.Height
	m.runCommand("/permission")

	if !m.onBoard() {
		t.Fatal("permission did not open a board")
	}
	if m.viewport.Height != tall {
		t.Errorf("a board took %d rows from the transcript; it replaces the screen instead",
			tall-m.viewport.Height)
	}

	view := stripANSI(m.View())
	if strings.Contains(view, "Ask something") {
		t.Errorf("the composer is still on screen behind the board:\n%s", view)
	}
	if !strings.Contains(view, "esc cancel") {
		t.Errorf("the board does not say how to leave:\n%s", view)
	}
}

// A screen that only repeats the option names has not earned itself, so every
// option carries something the one-line summary does not say.
func TestEveryOptionSaysMoreThanItsName(t *testing.T) {
	for _, line := range []string{"/vim", "/thinking", "/model", "/provider", "/clear"} {
		t.Run(line, func(t *testing.T) {
			m := chooserModel(t)
			m.agent.Provider = listingProvider{}
			m.messages = []llm.Message{llm.UserText("something")}
			m.runCommand(line)

			for i := range m.chooser.options {
				m.chooser.cursor = i
				o := m.chooser.options[i]
				if len(o.body) == 0 && len(o.grid) == 0 {
					t.Errorf("%s option %q shows nothing beyond its name", line, o.label)
				}
				if view := stripANSI(m.boardView()); !strings.Contains(view, o.label) {
					t.Errorf("%s does not draw option %q", line, o.label)
				}
			}
		})
	}
}

func TestTheBoardShowsTheHighlightedOptionsDetail(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/vim")

	m.chooser.cursor = 0
	on := stripANSI(m.boardView())
	m.chooser.cursor = 1
	off := stripANSI(m.boardView())

	if on == off {
		t.Fatal("the panel does not follow the cursor")
	}
	if !strings.Contains(on, "motions") {
		t.Errorf("the vim key map is not shown for \"on\":\n%s", on)
	}
	if strings.Contains(off, "motions") {
		t.Errorf("the vim key map is shown for \"off\":\n%s", off)
	}
}

// The bar travels the lines between two options rather than jumping the gap,
// marking each one as it passes.
func TestTheBarTravelsBetweenOptions(t *testing.T) {
	m := chooserModel(t)
	m.UseAnimation(true)
	m.runCommand("/vim")

	from := m.chooser.barAt
	m.moveChooser(1)
	to := m.chooser.barTo

	if to == from {
		t.Fatal("the options share a line, so there is nothing to travel")
	}
	if m.chooser.barAt != from {
		t.Error("the bar arrived before a single frame had run")
	}

	seen := map[int]bool{}
	for range 200 {
		seen[m.chooser.barAt] = true
		if _, cmd := m.Update(frameMsg{}); cmd == nil {
			break
		}
	}

	if m.chooser.barAt != to {
		t.Errorf("the bar stopped at line %d, want %d", m.chooser.barAt, to)
	}
	for line := min(from, to); line <= max(from, to); line++ {
		if !seen[line] {
			t.Errorf("the bar skipped line %d on its way from %d to %d", line, from, to)
		}
	}
	if len(m.chooser.trail) != 0 {
		t.Errorf("the trail never faded: %v", m.chooser.trail)
	}
}

// With animation off there is nothing to travel through, so it is simply there.
func TestTheBarArrivesAtOnceWithAnimationOff(t *testing.T) {
	m := chooserModel(t)
	m.UseAnimation(false)
	m.runCommand("/vim")

	m.moveChooser(1)
	if m.chooser.barAt != m.chooser.barTo {
		t.Errorf("bar at %d, want %d", m.chooser.barAt, m.chooser.barTo)
	}
}

// Two options with nothing else on the row say their name and their meaning
// together, with air between them.
func TestTwoOptionBoardsUseOneLineEach(t *testing.T) {
	m := chooserModel(t)
	m.UseAnimation(false)
	m.runCommand("/vim")

	view := stripANSI(m.boardView())
	if !strings.Contains(view, "on — starts in insert") {
		t.Errorf("the name and its meaning are not on one line:\n%s", view)
	}

	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "on — starts") {
			continue
		}
		if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) != "" {
			t.Errorf("no blank line between the options:\n%s", view)
		}
		return
	}
}

// An option carrying a grid still needs its own line for the detail.
func TestGridOptionsKeepTheirSecondLine(t *testing.T) {
	m := chooserModel(t)
	m.runCommand("/permission")

	lines := optionLines(m.chooser.options)
	if lines[1]-lines[0] != 3 {
		t.Errorf("options are %d lines apart, want 3 (row, detail, blank)", lines[1]-lines[0])
	}
}
