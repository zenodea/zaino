package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) moveCursor(delta int) tea.Cmd {
	if len(m.entries) == 0 {
		return nil
	}

	at := m.cursor
	if at < 0 {
		at = len(m.entries)
		if delta > 0 {
			at = -1
		}
	}

	for next := at + delta; next >= 0 && next < len(m.entries); next += delta {
		if m.entries[next].render(m.contentWidth()) == "" {
			continue
		}
		m.leaveTrail(m.cursor)
		m.cursor = next
		m.rerender()
		return m.showCursor()
	}

	if delta > 0 && m.cursor >= 0 {
		m.clearCursor()
	}
	return nil
}

func (m *Model) clearCursor() {
	if m.cursor < 0 {
		return
	}
	m.cursor = -1
	m.rerender()
}

func (m *Model) selectedEntry() (entry, bool) {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return entry{}, false
	}
	return m.entries[m.cursor], true
}

func (m *Model) toggleSelected() bool {
	e, ok := m.selectedEntry()
	if !ok {
		return false
	}
	if strings.TrimSpace(m.input.Value()) != "" {
		return false
	}
	if e.kind != entryTool {
		return true
	}

	m.entries[m.cursor].expanded = !e.expanded
	m.rerender()
	m.showCursor()
	return true
}

func (m *Model) showCursor() tea.Cmd {
	if !m.ready || m.cursor < 0 || m.cursor >= len(m.tops) {
		return nil
	}

	top := m.tops[m.cursor]
	bottom := top + m.heights[m.cursor]

	target := m.viewport.YOffset
	switch {
	case top < m.viewport.YOffset:
		target = top
	case bottom > m.viewport.YOffset+m.viewport.Height:
		target = max(bottom-m.viewport.Height, 0)
	}

	if !m.motion.on {
		m.viewport.SetYOffset(target)
		return nil
	}

	m.motion.target = target
	m.motion.scrolling = target != m.viewport.YOffset
	m.motion.landing = landingFrames
	return m.animate()
}
