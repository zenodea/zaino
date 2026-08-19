package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Some commands answer rather than ask. A sheet is the screen they answer on:
// read-only, scrollable, and gone the moment you are done with it.
type sheet struct {
	open   bool
	title  string
	lines  []string
	offset int

	// Opened from a permission question, so the question's keys still answer.
	ask bool
}

func (m *Model) show(title string, lines []string) tea.Cmd {
	m.sheet = sheet{open: true, title: title, lines: lines}
	m.syncHeight()
	return nil
}

func (m *Model) closeSheet() {
	m.sheet = sheet{}
	m.syncHeight()
}

func (m *Model) sheetRows() int { return max(m.viewport.Height-2, 3) }

func (m *Model) handleSheetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "down", "j", "ctrl+n":
		m.scrollSheet(1)
	case "up", "k", "ctrl+p":
		m.scrollSheet(-1)
	case "pgdown", "ctrl+d", "ctrl+f", "space":
		m.scrollSheet(m.sheetRows())
	case "pgup", "ctrl+u", "ctrl+b":
		m.scrollSheet(-m.sheetRows())
	case "home", "g":
		m.sheet.offset = 0
	case "end", "G":
		m.scrollSheet(len(m.sheet.lines))
	case "esc", "q", "h", "enter", "ctrl+c":
		m.closeSheet()
	}
	return m, nil
}

func (m *Model) scrollSheet(delta int) {
	over := max(len(m.sheet.lines)-m.sheetRows(), 0)
	m.sheet.offset = min(max(m.sheet.offset+delta, 0), over)
}

func (m *Model) sheetView() string {
	rows := m.sheetRows()
	end := min(m.sheet.offset+rows, len(m.sheet.lines))

	lines := []string{" " + metaStyle.Render(m.sheet.title), ""}
	for _, line := range m.sheet.lines[m.sheet.offset:end] {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, " "+line)
	}

	// Padded to the full height so the rule and the footer stay where they are
	// on every screen, however little a sheet has to say.
	for len(lines) < rows+2 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) sheetFooter() string {
	back := "esc or q to go back"
	if m.sheet.ask {
		back = "y allow · a always · n refuse · esc back"
	}
	if len(m.sheet.lines) <= m.sheetRows() {
		return hintStyle.Render(back)
	}

	shown := min(m.sheet.offset+m.sheetRows(), len(m.sheet.lines))
	return hintStyle.Render("j/k or ↑↓ scroll · "+back) +
		metaStyle.Render("   "+humanTokens(shown)+" of "+humanTokens(len(m.sheet.lines))+" lines")
}

// A labelled bar. What the number means is easier to see as a length than as
// a digit, especially next to the other numbers it is competing with.
func bar(label string, value, total, width int, style lipgloss.Style) string {
	filled := 0
	if total > 0 {
		filled = min(value*width/total, width)
	}
	if value > 0 && filled == 0 {
		filled = 1
	}

	return pad(hintStyle.Render(label), 12) + " " +
		style.Render(strings.Repeat("█", filled)) +
		gutterStyle.Render(strings.Repeat("░", width-filled)) +
		"  " + metaStyle.Render(humanTokens(value))
}
