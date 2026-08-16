package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// secret is a masked one-line prompt. Keys are typed here rather than into
// the composer so they never reach the transcript, the session file, or the
// scrollback.
type secret struct {
	open  bool
	title string
	hint  string
	input textinput.Model

	apply func(m *Model, value string) tea.Cmd
}

func (m *Model) askSecret(title, hint string, apply func(*Model, string) tea.Cmd) tea.Cmd {
	in := textinput.New()
	in.EchoMode = textinput.EchoPassword
	in.EchoCharacter = '•'
	in.Prompt = ""
	in.CharLimit = 400
	in.Focus()

	m.secret = secret{open: true, title: title, hint: hint, input: in, apply: apply}
	return textinput.Blink
}

func (m *Model) closeSecret() {
	m.secret.input.Blur()
	m.secret = secret{}
}

func (m *Model) handleSecretKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.closeSecret()
		m.notice("cancelled")
		return m, nil

	case tea.KeyEnter:
		value := strings.TrimSpace(m.secret.input.Value())
		apply := m.secret.apply
		m.closeSecret()
		if value == "" {
			m.notice("nothing entered")
			return m, nil
		}
		if apply == nil {
			return m, nil
		}
		return m, apply(m, value)
	}

	var cmd tea.Cmd
	m.secret.input, cmd = m.secret.input.Update(msg)
	return m, cmd
}

func (m *Model) secretView() string {
	width := max(m.width-4, 20)

	title := brandStyle.Render(m.secret.title)
	field := lipgloss.NewStyle().
		Width(width).
		Render(m.secret.input.View())

	lines := []string{title, "", field}
	if m.secret.hint != "" {
		lines = append(lines, "", hintStyle.Render(m.secret.hint))
	}
	lines = append(lines, "", hintStyle.Render("⏎ save · esc cancel · the key is not echoed or recorded"))

	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(lines, "\n"))
}
