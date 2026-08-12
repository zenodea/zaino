package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) inVisualMode() bool { return m.inNormalMode() && m.vim.visual }

func (m *Model) enterVisual() {
	_, pos := m.buffer()
	m.vim.visual = true
	m.vim.anchor = pos
	m.vim.lineWise = false
}

func (m *Model) enterVisualLine() {
	m.enterVisual()
	m.vim.lineWise = true
}

func (m *Model) leaveVisual() {
	m.vim.visual = false
	m.vim.lineWise = false
}

func (m *Model) visualRange() (int, int) {
	text, pos := m.buffer()
	from, to := min(m.vim.anchor, pos), max(m.vim.anchor, pos)

	if m.vim.lineWise {
		from = lineStart(text, from)
		to = lineEnd(text, to)
		return from, to
	}
	return from, min(to+1, len(text))
}

func (m *Model) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	text, _ := m.buffer()
	from, to := m.visualRange()

	switch msg.String() {
	case "esc":
		m.leaveVisual()

	case "v":
		m.leaveVisual()

	case "V":
		m.vim.lineWise = !m.vim.lineWise

	case "y":
		m.vim.register = string(text[from:to])
		m.leaveVisual()
		m.setBuffer(text, from)

	case "d", "x":
		m.pushUndo()
		m.vim.register = string(text[from:to])
		m.leaveVisual()
		m.setBuffer(cut(text, from, to), from)

	case "c", "s":
		m.pushUndo()
		m.vim.register = string(text[from:to])
		m.leaveVisual()
		m.enterInsert()
		m.setBuffer(cut(text, from, to), from)

	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m *Model) visualView() string {
	text, _ := m.buffer()
	from, to := m.visualRange()

	spans := []span{
		{text: string(text[:from]), style: userStyle},
		{text: string(text[from:to]), style: visualStyle},
		{text: string(text[to:]), style: userStyle},
	}
	if to >= len(text) {
		spans[2].text = " "
		spans[2].style = visualStyle
	}

	width := max(m.contentWidth()-gutterWidth, 20)
	return strings.Join(wrapSpans(spans, width, "", ""), "\n")
}
