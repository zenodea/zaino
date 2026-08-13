package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/session"
)

type picker struct {
	open   bool
	items  []session.Summary
	cursor int
}

func (m *Model) openPicker() {
	if m.repo == nil {
		m.notice("sessions are not being saved")
		return
	}

	items, err := m.repo.List()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}
	if len(items) == 0 {
		m.notice("no saved sessions yet")
		return
	}

	slices.Reverse(items)

	m.picker = picker{open: true, items: items, cursor: len(items) - 1}
	for i, s := range items {
		if s.ID == m.sessionID() {
			m.picker.cursor = i
			break
		}
	}
	m.syncViewport()
}

func (m *Model) closePicker() {
	m.picker = picker{}
	m.syncViewport()
}

func (m *Model) handlePickerKey(msg tea.KeyMsg) tea.Cmd {
	switch key := msg.String(); key {
	case "up", "k", "ctrl+p", "shift+tab":
		m.movePicker(-1)
	case "down", "j", "ctrl+n", "tab":
		m.movePicker(1)
	case "pgup", "ctrl+u", "ctrl+b":
		m.movePicker(-m.pickerRows())
	case "pgdown", "ctrl+d", "ctrl+f":
		m.movePicker(m.pickerRows())
	case "home", "g":
		m.movePicker(-len(m.picker.items))
	case "end", "G":
		m.movePicker(len(m.picker.items))
	case "enter", "l", "o":
		if m.picker.cursor < len(m.picker.items) {
			id := m.picker.items[m.picker.cursor].ID
			m.closePicker()
			m.resume(id)
		}
	case "esc", "q", "h", "ctrl+c":
		m.closePicker()
	}
	return nil
}

func (m *Model) movePicker(delta int) {
	n := len(m.picker.items)
	if n == 0 {
		return
	}
	m.picker.cursor = min(max(m.picker.cursor+delta, 0), n-1)
	m.syncViewport()
}

func (m *Model) resume(id string) {
	if m.repo == nil {
		return
	}
	if id == m.sessionID() {
		m.notice("already in %s", id)
		return
	}

	store, err := m.repo.Open(id)
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}
	entries, err := store.Entries()
	if err != nil {
		store.Close()
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}

	ctx := session.Build(entries)

	switched := m.provider
	if ctx.Provider != "" && ctx.Provider != m.provider {
		backend, err := provider.New(ctx.Provider)
		if err != nil {
			m.notice("session used %s, staying on %s: %s", ctx.Provider, m.provider, err)
		} else {
			m.agent.Provider = backend
			m.provider = backend.Name()
			switched = backend.Name()
		}
	}

	var stripped int
	if ctx.Provider != "" && ctx.Provider != switched {
		ctx.Messages, stripped = session.StripProviderBlocks(ctx.Messages)
	}

	m.agent.Model = ctx.Model
	m.agent.System = ctx.System
	m.agent.Effort = ctx.Effort
	if ctx.Thinking != nil {
		if m.agent.Thinking == nil {
			m.agent.Thinking = &llm.Thinking{Enabled: true}
		}
		m.agent.Thinking.Show = *ctx.Thinking
	}
	m.lastModel = ""

	m.rec.Close()
	m.rec.Use(store, len(ctx.Messages))
	m.saveFailed = false

	m.Restore(ctx)
	if stripped > 0 {
		m.notice("dropped %d reasoning blocks that only %s can read back", stripped, ctx.Provider)
	}
}

const pickerChrome = 2

func (m *Model) pickerRows() int {
	return max(m.viewport.Height-pickerChrome, 3)
}

func (m *Model) pickerView() string {
	rows := m.pickerRows()
	items, cursor := m.pickerWindow(rows)

	lines := make([]string, 0, len(items)+2)
	lines = append(lines, m.pickerHeading())
	lines = append(lines, "")

	for i, s := range items {
		marker, style := " ", metaStyle
		if i == cursor {
			marker, style = "›", menuPickStyle
		}
		here := "  "
		if s.ID == m.sessionID() {
			here = "· "
		}

		left := fmt.Sprintf("%s%-16s %6s %8s  ",
			here, when(s.Updated), fmt.Sprintf("%dmsg", s.Messages), humanTokens(s.Tokens))
		preview := s.Preview
		if preview == "" {
			preview = "(nothing said)"
		}
		room := max(m.contentWidth()-lipgloss.Width(left)-4, 12)

		lines = append(lines,
			userMarker.Render(marker)+" "+style.Render(left+truncate(preview, room)))
	}

	if pad := m.viewport.Height - len(lines); pad > 0 {
		lines = append(make([]string, pad), lines...)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) pickerWindow(rows int) ([]session.Summary, int) {
	if len(m.picker.items) <= rows {
		return m.picker.items, m.picker.cursor
	}
	start := min(max(m.picker.cursor-rows/2, 0), len(m.picker.items)-rows)
	return m.picker.items[start : start+rows], m.picker.cursor - start
}

func (m *Model) pickerHeading() string {
	return metaStyle.Render(fmt.Sprintf("sessions · %d of %d",
		m.picker.cursor+1, len(m.picker.items)))
}

func when(t time.Time) string {
	switch since := time.Since(t); {
	case since < time.Minute:
		return "just now"
	case since < time.Hour:
		return fmt.Sprintf("%dm ago", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(since.Hours()))
	case since < 7*24*time.Hour:
		return t.Format("Mon 15:04")
	default:
		return t.Format("2006-01-02 15:04")
	}
}
