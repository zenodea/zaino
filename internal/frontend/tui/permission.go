package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/store/session"
)

type askMsg struct {
	req   permission.Request
	reply chan permission.Grant
}

type pendingAsk struct {
	req   permission.Request
	reply chan permission.Grant
}

type approver struct {
	events chan tea.Msg
}

// The agent runs on its own goroutine and cannot draw: the question goes into
// the same channel the hooks use, and the answer comes back down a reply chan.
func (a *approver) Approve(ctx context.Context, req permission.Request) (permission.Grant, error) {
	reply := make(chan permission.Grant, 1)
	select {
	case a.events <- askMsg{req: req, reply: reply}:
	case <-ctx.Done():
		return permission.Reject, ctx.Err()
	}

	select {
	case grant := <-reply:
		return grant, nil
	case <-ctx.Done():
		return permission.Reject, ctx.Err()
	}
}

func (m *Model) Approver() permission.Approver { return &approver{events: m.events} }

func (m *Model) answer(grant permission.Grant) {
	if m.pending == nil {
		return
	}
	req := m.pending.req
	m.pending.reply <- grant
	m.pending = nil
	m.record(session.Permission(req.Tool, string(req.Action), req.Target, decisionName(grant)))
}

func decisionName(grant permission.Grant) string {
	switch grant {
	case permission.Once:
		return "allowed"
	case permission.Always:
		return "allowed-always"
	}
	return "refused"
}

func (m *Model) handleAskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.answer(permission.Once)
		return m, nil

	case "a":
		m.answer(permission.Always)
		m.push(entry{kind: entryNotice, text: "allowed for the rest of this session"})
		return m, nil

	case "n", "esc":
		m.answer(permission.Reject)
		return m, nil

	// Refusing before cancelling matters: cancelling while the agent still
	// waits on the reply would leave it blocked.
	case "ctrl+c":
		m.answer(permission.Reject)
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) askView() string {
	if m.pending == nil {
		return ""
	}
	req := m.pending.req

	inner := max(m.contentWidth()-4, 24)
	divider := gutterStyle.Render(strings.Repeat("╌", inner))

	lines := []string{askHeader(req, inner)}
	if preview := previewLines(req, inner); len(preview) > 0 {
		lines = append(lines, divider)
		lines = append(lines, preview...)
	}
	lines = append(lines, divider, choices(req, inner))

	return askBoxStyle.Render(strings.Join(lines, "\n"))
}

func askHeader(req permission.Request, width int) string {
	left := keyCapStyle.Render(verb(req.Action)) + " " + bodyStyle.Render(req.Target)
	right := metaStyle.Render(req.Tool)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clamp(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func choices(req permission.Request, width int) string {
	scope := "this file"
	if req.Action == permission.Execute {
		scope = program(req.Target)
	}

	for _, set := range [][2]string{
		{"allow once", "always allow " + scope},
		{"allow", "always allow " + scope},
		{"allow", "always"},
	} {
		line := strings.Join([]string{
			keyHint("y", set[0]),
			keyHint("a", set[1]),
			keyHint("n", "refuse"),
			keyHint("⌃c", "refuse and stop"),
		}, metaStyle.Render("  ·  "))
		if lipgloss.Width(line) <= width {
			return line
		}
	}
	return clamp(strings.Join([]string{
		keyHint("y", "allow"),
		keyHint("a", "always"),
		keyHint("n", "refuse"),
	}, metaStyle.Render(" · ")), width)
}

func keyHint(key, what string) string {
	return keyCapStyle.Render(key) + " " + hintStyle.Render(what)
}

func program(command string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(command), " ")
	if first == "" {
		return "this command"
	}
	return first
}

const previewHeight = 16

func previewLines(req permission.Request, width int) []string {
	lines := strings.Split(strings.TrimRight(req.Preview, "\n"), "\n")

	hidden := 0
	if len(lines) > previewHeight {
		hidden = len(lines) - previewHeight
		lines = lines[:previewHeight]
	}

	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		out = append(out, paintDiffLine(clamp(line, width), req.Action))
	}
	if hidden > 0 {
		out = append(out, metaStyle.Render(fmt.Sprintf("… %d more lines", hidden)))
	}
	return out
}

func paintDiffLine(line string, action permission.Action) string {
	if action == permission.Execute {
		return mdCodeStyle.Render(line)
	}

	number, rest, ok := splitDiffLine(line)
	if !ok {
		return metaStyle.Render(line)
	}
	switch {
	case strings.HasPrefix(rest, "+"):
		return gutterStyle.Render(number) + addStyle.Render(rest)
	case strings.HasPrefix(rest, "-"):
		return gutterStyle.Render(number) + removeStyle.Render(rest)
	}
	return gutterStyle.Render(number) + metaStyle.Render(rest)
}

func splitDiffLine(line string) (number, rest string, ok bool) {
	at := 0
	for at < len(line) && line[at] == ' ' {
		at++
	}
	start := at
	for at < len(line) && line[at] >= '0' && line[at] <= '9' {
		at++
	}
	if at == start || at >= len(line) || line[at] != ' ' {
		return "", "", false
	}
	return line[:at+1], line[at+1:], true
}

func verb(action permission.Action) string {
	switch action {
	case permission.Write:
		return "Write"
	case permission.Execute:
		return "Run"
	case permission.Network:
		return "Fetch"
	}
	return "Read"
}
