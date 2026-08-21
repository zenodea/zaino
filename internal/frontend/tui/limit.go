package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/agent"
)

// A turn held at the ceiling. Nothing was sent, and the conversation is intact,
// so the only question left is whether to send it anyway.
type limitGate struct {
	open  bool
	used  int
	limit int
	exact bool

	// The other way a turn is held: it ran to its limit of tool rounds,
	// and the question is whether to let it carry on.
	turns bool
}

func (m *Model) holdAtLimit(err *agent.ContextLimitError) {
	m.limit = limitGate{open: true, used: err.Used, limit: err.Limit, exact: err.Exact}
}

func (m *Model) holdAtTurns() {
	limit := m.agent.MaxTurns
	if limit <= 0 {
		limit = agent.DefaultMaxTurns
	}
	m.limit = limitGate{open: true, turns: true, limit: limit}
}

func (m *Model) handleLimitKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		held := m.limit
		m.limit = limitGate{}
		if held.turns {
			m.push(entry{kind: entryNotice, text: fmt.Sprintf("carrying on for another %d rounds", held.limit)})
			return m, m.launch()
		}
		m.push(entry{kind: entryNotice, text: fmt.Sprintf("sent past the %s limit",
			humanTokens(m.agent.MaxContext))})
		m.agent.AllowOnce()
		return m, m.launch()

	case "esc", "ctrl+c", "q", "n":
		held := m.limit
		m.limit = limitGate{}
		if held.turns {
			m.push(entry{kind: entryNotice, text: "left there · ask again to pick it up, or say what to do next"})
			return m, nil
		}
		m.push(entry{kind: entryNotice, text: "turn not sent · /limit raises the ceiling, /compact makes room"})
		return m, nil
	}
	return m, nil
}

func (m *Model) limitView() string {
	if !m.limit.open {
		return ""
	}

	inner := max(m.contentWidth()-4, 24)
	divider := gutterStyle.Render(strings.Repeat("╌", inner))

	var head, said, yes string
	if m.limit.turns {
		head = errorMarker.Render("⚠ turn limit") + " " +
			bodyStyle.Render(fmt.Sprintf("%d rounds of tool calls without finishing", m.limit.limit))
		said = "the model is still working; nothing has been lost."
		yes = "let it carry on"
	} else {
		about := "about "
		if m.limit.exact {
			about = ""
		}
		head = errorMarker.Render("⚠ context limit") + " " +
			bodyStyle.Render(about+humanTokens(m.limit.used)+" of "+humanTokens(m.limit.limit))
		right := metaStyle.Render(source(m.limit.exact, m.provider))
		gap := inner - lipgloss.Width(head) - lipgloss.Width(right)
		if gap < 1 {
			head = clamp(head, inner)
		} else {
			head += strings.Repeat(" ", gap) + right
		}
		said = "the turn was stopped before it was sent."
		yes = "send it anyway"
	}

	return askBoxStyle.Render(strings.Join([]string{
		head,
		divider,
		hintStyle.Render(said),
		divider,
		strings.Join([]string{
			keyHint("⏎", yes),
			keyHint("esc", "leave it here"),
		}, metaStyle.Render("  ·  ")),
	}, "\n"))
}

func source(exact bool, provider string) string {
	if exact {
		return "counted by " + provider
	}
	return "estimated here"
}
