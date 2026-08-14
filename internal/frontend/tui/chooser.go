package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// How much room a choice earns. A boolean does not deserve a screen; a setting
// that decides what the agent may do to your machine does.
type layout int

const (
	layoutBoard layout = iota // a screen, with what each option means on it
	layoutScale               // one dimension, so it is drawn as one
)

type gridCell struct {
	label string
	state int
}

const (
	stateRefuse = iota
	stateAsk
	stateAllow
)

type choice struct {
	label  string
	detail string
	value  string

	// Where this option sits on an ordered scale, 1 upwards.
	level int

	// What this option means, shown under the list while it is highlighted.
	// A screen that only repeats the option names has not earned itself.
	grid []gridCell
	body []string
}

// A command run with no argument is a question, so it gets asked rather than
// answered with a sentence about what the options are.
type chooser struct {
	open    bool
	layout  layout
	title   string
	options []choice
	cursor  int
	current string
	apply   func(m *Model, picked choice)

	// Cells of the highlighted option's meter drawn so far, and the frames
	// spent on the current one.
	fill int
	tick int

	// The row you came from, fading, as in the transcript.
	trail map[int]int
}

const (
	framesPerCell      = 4
	chooserFrameHeight = 4
	scaleFrameHeight   = 9
)

func (m *Model) ask(c chooser) tea.Cmd {
	if len(c.options) == 0 {
		return nil
	}

	c.open = true
	for i, o := range c.options {
		if o.value == c.current {
			c.cursor = i
			break
		}
	}
	m.chooser = c
	m.syncHeight()
	return m.startFill()
}

func (m *Model) closeChooser() {
	m.chooser = chooser{}
	m.syncHeight()
}

func (m *Model) handleChooserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A horizontal control takes horizontal keys as well, never instead: an
	// arrow that does nothing is worse than one that does the obvious thing.
	scale := m.chooser.layout == layoutScale

	switch key := msg.String(); {
	case key == "up", key == "k", key == "ctrl+p", key == "shift+tab",
		scale && (key == "left" || key == "h"):
		return m, m.moveChooser(-1)

	case key == "down", key == "j", key == "ctrl+n", key == "tab",
		scale && (key == "right" || key == "l"):
		return m, m.moveChooser(1)

	case key == "home", key == "g":
		return m, m.moveChooser(-m.chooser.cursor)

	case key == "end", key == "G":
		return m, m.moveChooser(len(m.chooser.options) - 1 - m.chooser.cursor)

	case key == "enter", key == "l":
		picked := m.chooser.options[m.chooser.cursor]
		apply := m.chooser.apply
		m.closeChooser()
		if apply != nil {
			apply(m, picked)
		}

	case key == "esc", key == "q", key == "h", key == "ctrl+c":
		m.closeChooser()
	}
	return m, nil
}

func (m *Model) moveChooser(delta int) tea.Cmd {
	n := len(m.chooser.options)
	if n == 0 || delta == 0 {
		return nil
	}
	m.leaveChooserTrail(m.chooser.cursor)
	m.chooser.cursor = (m.chooser.cursor + delta + n) % n
	m.chooser.fill, m.chooser.tick = 0, 0
	return m.startFill()
}

func (m *Model) leaveChooserTrail(from int) {
	if !m.motion.on {
		return
	}
	if m.chooser.trail == nil {
		m.chooser.trail = map[int]int{}
	}
	for i, life := range m.chooser.trail {
		if life -= framesPerShade; life <= 0 {
			delete(m.chooser.trail, i)
			continue
		}
		m.chooser.trail[i] = life
	}
	m.chooser.trail[from] = trailLife()
}

func (m *Model) fadeChooserTrail() bool {
	for i := range m.chooser.trail {
		if m.chooser.trail[i]--; m.chooser.trail[i] <= 0 {
			delete(m.chooser.trail, i)
		}
	}
	return len(m.chooser.trail) > 0
}

func (m *Model) chooserBar(i int) string {
	if i == m.chooser.cursor {
		return cursorBar()
	}
	if life, ok := m.chooser.trail[i]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return " "
}

// With animation off the meter is drawn full: an empty one would read as
// "none", which is the wrong answer.
func (m *Model) startFill() tea.Cmd {
	if !m.motion.on {
		m.chooser.fill = m.fillTarget()
		return nil
	}
	return m.animate()
}

func (m *Model) fillTarget() int {
	if m.chooser.layout == layoutScale {
		return m.chooser.cursor + 1
	}
	return m.chooser.options[m.chooser.cursor].level
}

// The meter under the cursor fills a cell at a time, so moving up the scale
// looks like turning something up.
func (m *Model) fillMeter() bool {
	if !m.chooser.open {
		return false
	}
	fading := m.fadeChooserTrail()
	if m.chooser.fill >= m.fillTarget() {
		return fading
	}
	if m.chooser.tick++; m.chooser.tick >= framesPerCell {
		m.chooser.fill, m.chooser.tick = m.chooser.fill+1, 0
	}
	return true
}

// A board takes the whole screen rather than a slice of it, so it costs the
// transcript nothing to measure.
func (m *Model) chooserHeight() int {
	if m.chooser.open && m.chooser.layout == layoutScale {
		return scaleFrameHeight
	}
	return 0
}

func (m *Model) onBoard() bool { return m.chooser.open && m.chooser.layout == layoutBoard }

func (m *Model) chooserView() string {
	if m.chooser.open && m.chooser.layout == layoutScale {
		return m.scaleView()
	}
	return ""
}

// A scale laid across the screen: the columns light up to where you are, so a
// setting on a dial reads as a level rather than as a word in a list.
func (m *Model) scaleView() string {
	room := max(m.contentWidth()-2, 30)

	column := 0
	for _, o := range m.chooser.options {
		column = max(column, lipgloss.Width(o.label)+2)
	}
	// Every stop has to be on screen, or the scale is lying about its range.
	column = min(column, (room-2)/len(m.chooser.options))
	inner := min(column*len(m.chooser.options)+2, room)

	var names, bars, mark strings.Builder
	for i, o := range m.chooser.options {
		style := metaStyle
		if i == m.chooser.cursor {
			style = menuPickStyle
		}
		names.WriteString(style.Render(centre(clamp(o.label, column), column)))
		bars.WriteString(centre(scaleCell(o.level, i < m.chooser.fill), column))

		if i == m.chooser.cursor {
			mark.WriteString(menuPickStyle.Render(centre("▲", column)))
			continue
		}
		mark.WriteString(strings.Repeat(" ", column))
	}

	picked := m.chooser.options[m.chooser.cursor]
	foot := picked.label
	if picked.detail != "" {
		foot += " · " + picked.detail
	}

	lines := []string{
		metaStyle.Render(m.chooser.title), "",
		clamp(names.String(), inner-2),
		clamp(bars.String(), inner-2),
		clamp(mark.String(), inner-2),
		"",
		hintStyle.Render(clamp(foot, inner-2)),
	}
	return menuBoxStyle.Width(inner).Render(strings.Join(lines, "\n"))
}

// The grid says what each mode actually permits. It is asked of the policy
// rather than written down, so it cannot drift from what the gate will do.
func (m *Model) boardView() string {
	label, name := 0, 0
	for _, o := range m.chooser.options {
		label = max(label, lipgloss.Width(o.label))
		for _, g := range o.grid {
			name = max(name, lipgloss.Width(g.label))
		}
	}

	lines := []string{" " + metaStyle.Render(m.chooser.title), ""}
	for i, o := range m.chooser.options {
		style := metaStyle
		if i == m.chooser.cursor {
			style = menuPickStyle
		}
		here := "  "
		if o.value == m.chooser.current {
			here = "· "
		}

		// The label is padded before the mark, so the marks form a column and
		// two options can be compared down the screen.
		var grid strings.Builder
		for _, g := range o.grid {
			grid.WriteString(hintStyle.Render(pad(g.label, name)) + " " + stateMark(g.state) + "   ")
		}

		row := " " + m.chooserBar(i) + " " + here + style.Render(pad(o.label, label+2)) + grid.String()
		lines = append(lines, clamp(strings.TrimRight(row, " "), m.contentWidth()))
		if o.detail != "" {
			lines = append(lines, "     "+hintStyle.Render(clamp(o.detail, m.contentWidth()-6)))
		}
	}

	if body := m.chooser.options[m.chooser.cursor].body; len(body) > 0 {
		lines = append(lines, "", " "+gutterStyle.Render(strings.Repeat("╌", max(m.contentWidth()-2, 10))), "")
		for _, line := range body {
			lines = append(lines, " "+clamp(line, m.contentWidth()-2))
		}
	}
	return strings.Join(lines, "\n")
}

func centre(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	left := gap / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
}

func pad(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}
