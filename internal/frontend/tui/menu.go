package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	menuMaxRows = 6

	// The blank line holding the panel off the rule, plus its border.
	menuFrameHeight = 3
)

type menu struct {
	open    bool
	matches []command
	cursor  int

	// The bar walks between rows here for the same reason it does in the
	// transcript: you can see where it went.
	barAt int
	barTo int
	step  int
	trail map[int]int
}

func (m *Model) refreshMenu() {
	value := m.input.Value()
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
		m.menu = menu{}
		return
	}

	selected := ""
	if m.menu.open && m.menu.cursor < len(m.menu.matches) {
		selected = m.menu.matches[m.menu.cursor].name
	}

	matches := rankCommands(m.available(), strings.TrimPrefix(value, "/"))
	if len(matches) == 0 {
		m.menu = menu{}
		return
	}

	cursor := 0
	for i, c := range matches {
		if c.name == selected {
			cursor = i
			break
		}
	}

	// Narrowing the list must not teleport the bar, so its place carries over
	// from the menu that was already open.
	was := m.menu
	m.menu = menu{open: true, matches: matches, cursor: cursor,
		barAt: was.barAt, trail: was.trail}

	_, row := m.window()
	m.menu.barTo = row
	if !was.open || !m.motion.on {
		m.menu.barAt = row
	}
}

func (m *Model) moveMenu(delta int) tea.Cmd {
	n := len(m.menu.matches)
	if n == 0 {
		return nil
	}
	m.menu.cursor = (m.menu.cursor + delta + n) % n
	m.menu.step = 0

	_, row := m.window()
	m.menu.barTo = row
	if !m.motion.on {
		m.leaveMenuTrail(m.menu.barAt)
		m.menu.barAt = row
		return nil
	}
	return m.animate()
}

func (m *Model) menuBar(row int) string {
	if row == m.menu.barAt {
		return cursorBar()
	}
	if life, ok := m.menu.trail[row]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return noBar()
}

func (m *Model) leaveMenuTrail(row int) {
	if m.menu.trail == nil {
		m.menu.trail = map[int]int{}
	}
	m.menu.trail[row] = trailLife()
}

// advanceMenuBar walks the bar a line at a time and fades what it leaves.
func (m *Model) advanceMenuBar() bool {
	if !m.menu.open {
		return false
	}
	moving := false
	if m.menu.barAt != m.menu.barTo {
		if m.menu.step++; m.menu.step >= framesPerLine {
			m.menu.step = 0
			m.leaveMenuTrail(m.menu.barAt)
			if m.menu.barAt < m.menu.barTo {
				m.menu.barAt++
			} else {
				m.menu.barAt--
			}
		}
		moving = true
	}
	return m.fadeMenuTrail() || moving
}

func (m *Model) fadeMenuTrail() bool {
	for row := range m.menu.trail {
		if m.menu.trail[row]--; m.menu.trail[row] <= 0 {
			delete(m.menu.trail, row)
		}
	}
	return len(m.menu.trail) > 0
}

func (m *Model) selected() (command, bool) {
	if !m.menu.open || m.menu.cursor >= len(m.menu.matches) {
		return command{}, false
	}
	return m.menu.matches[m.menu.cursor], true
}

func (m *Model) complete() {
	c, ok := m.selected()
	if !ok {
		return
	}
	line := "/" + c.name
	if c.arg != "" {
		line += " "
	}
	m.input.SetValue(line)
	m.input.CursorEnd()
	m.syncInputChrome()
	if c.arg == "" {
		m.menu = menu{}
		m.syncHeight()
	}
}

func (m *Model) rowLimit() int {
	if m.height <= 0 {
		return menuMaxRows
	}
	// header, blank, a usable viewport, the frame, the rule, input, footer
	chrome := 1 + 1 + 3 + menuFrameHeight + 1 + m.input.Height() + 1
	return min(menuMaxRows, max(m.height-chrome, 1))
}

func (m *Model) window() ([]command, int) {
	limit := m.rowLimit()
	if len(m.menu.matches) <= limit {
		return m.menu.matches, m.menu.cursor
	}
	start := min(max(m.menu.cursor-limit/2, 0), len(m.menu.matches)-limit)
	return m.menu.matches[start : start+limit], m.menu.cursor - start
}

func (m *Model) menuHeight() int {
	if !m.menu.open {
		return 0
	}
	rows, _ := m.window()
	return len(rows) + menuFrameHeight
}

func (m *Model) menuView() string {
	if !m.menu.open {
		return ""
	}
	rows, cursor := m.window()
	names, inner := m.menuColumns()
	inner = min(inner, max(m.contentWidth()-2, 28))
	summaries := max(inner-menuGutter-names, 8)

	lines := make([]string, len(rows))
	for i, c := range rows {
		name := "/" + c.name
		gap := strings.Repeat(" ", names-len(name)+2)

		label, summary := metaStyle, metaStyle
		if i == cursor {
			label, summary = menuPickStyle, hintStyle
		}
		lines[i] = m.menuBar(i) + " " +
			label.Render(name) + gap + summary.Render(truncate(c.summary, summaries))
	}
	return menuBoxStyle.Width(inner).Render(strings.Join(lines, "\n"))
}

// Box padding, the marker and its space, the gap between the columns.
const menuGutter = 2 + 2 + 2

// Sized off the whole registry, not the visible matches: every summary fits
// whole, and the box holds still while the list narrows under the cursor.
func (m *Model) menuColumns() (names, inner int) {
	summaries := 0
	for _, c := range m.commands() {
		names = max(names, len("/"+c.name))
		summaries = max(summaries, len(c.summary))
	}
	return names, names + summaries + menuGutter
}
