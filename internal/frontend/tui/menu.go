package tui

import "strings"

const (
	menuMaxRows = 6

	// The blank line holding the panel off the rule, plus its border.
	menuFrameHeight = 3
)

type menu struct {
	open    bool
	matches []command
	cursor  int
}

func (m *Model) refreshMenu() {
	value := m.input.Value()
	if m.streaming || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
		m.menu = menu{}
		return
	}

	selected := ""
	if m.menu.open && m.menu.cursor < len(m.menu.matches) {
		selected = m.menu.matches[m.menu.cursor].name
	}

	matches := matchCommands(strings.TrimPrefix(value, "/"))
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
	m.menu = menu{open: true, matches: matches, cursor: cursor}
}

func (m *Model) moveMenu(delta int) {
	n := len(m.menu.matches)
	if n == 0 {
		return
	}
	m.menu.cursor = (m.menu.cursor + delta + n) % n
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
	names, inner := menuColumns()
	inner = min(inner, max(m.contentWidth()-2, 28))
	summaries := max(inner-menuGutter-names, 8)

	lines := make([]string, len(rows))
	for i, c := range rows {
		name := "/" + c.name
		gap := strings.Repeat(" ", names-len(name)+2)

		marker, label, summary := " ", metaStyle, metaStyle
		if i == cursor {
			marker, label, summary = "›", menuPickStyle, hintStyle
		}
		lines[i] = userMarker.Render(marker) + " " +
			label.Render(name) + gap + summary.Render(truncate(c.summary, summaries))
	}
	return menuBoxStyle.Width(inner).Render(strings.Join(lines, "\n"))
}

// Box padding, the marker and its space, the gap between the columns.
const menuGutter = 2 + 2 + 2

// Sized off the whole registry, not the visible matches: every summary fits
// whole, and the box holds still while the list narrows under the cursor.
func menuColumns() (names, inner int) {
	summaries := 0
	for _, c := range commandList() {
		names = max(names, len("/"+c.name))
		summaries = max(summaries, len(c.summary))
	}
	return names, names + summaries + menuGutter
}
