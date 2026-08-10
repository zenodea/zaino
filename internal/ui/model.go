package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
)

type (
	textDeltaMsg  string
	thinkDeltaMsg string
	toolCallMsg   struct{ Call llm.ToolUseBlock }
	toolResultMsg struct {
		Call    llm.ToolUseBlock
		Result  string
		IsError bool
	}
	turnMsg struct {
		Model string
		Usage llm.Usage
		Stop  llm.StopReason
	}
	doneMsg struct {
		History []llm.Message
		Err     error
	}
)

const (
	maxInputHeight = 6
	argsLimit      = 72
)

type Model struct {
	agent    *agent.Agent
	provider string

	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	entries  []entry
	rendered []string
	history  []llm.Message

	events chan tea.Msg
	cancel context.CancelFunc

	width, height int
	ready         bool

	streaming    bool
	startedAt    time.Time
	sessionUsage llm.Usage
	lastModel    string

	quitting bool
}

func New(ag *agent.Agent, providerName string) *Model {
	input := textarea.New()
	input.Placeholder = "Ask something…"
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.CharLimit = 0
	input.SetHeight(1)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Base = lipgloss.NewStyle()
	input.BlurredStyle.Base = lipgloss.NewStyle()
	input.Focus()

	input.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(spinnerStyle))

	return &Model{
		agent:    ag,
		provider: providerName,
		input:    input,
		spinner:  sp,
		events:   make(chan tea.Msg, 64),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick, m.waitForEvent())
}

func (m *Model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return <-m.events }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case textDeltaMsg:
		m.appendToOpenEntry(entryAssistant, string(msg))
		return m, m.waitForEvent()

	case thinkDeltaMsg:
		m.appendToOpenEntry(entryThinking, string(msg))
		return m, m.waitForEvent()

	case toolCallMsg:
		m.push(entry{
			kind:     entryTool,
			toolName: msg.Call.Name,
			toolArgs: compactArgs(msg.Call.Input, argsLimit),
		})
		return m, m.waitForEvent()

	case toolResultMsg:
		m.completeTool(msg)
		return m, m.waitForEvent()

	case turnMsg:
		m.lastModel = msg.Model
		m.sessionUsage.InputTokens += msg.Usage.InputTokens
		m.sessionUsage.OutputTokens += msg.Usage.OutputTokens
		m.sessionUsage.ThinkingTokens += msg.Usage.ThinkingTokens
		m.sessionUsage.CacheReadTokens += msg.Usage.CacheReadTokens
		m.sessionUsage.CacheWriteTokens += msg.Usage.CacheWriteTokens
		return m, m.waitForEvent()

	case doneMsg:
		m.finishTurn(msg)
		return m, m.waitForEvent()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.growInput()
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":

		if m.streaming && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case "ctrl+d":
		m.quitting = true
		return m, tea.Quit

	case "enter":
		if m.streaming {
			return m, nil
		}
		return m, m.submit()

	case "ctrl+j":
		m.input.InsertString("\n")
		m.growInput()
		return m, nil

	case "ctrl+l":
		m.reset()
		return m, nil

	case "pgup", "pgdown", "ctrl+u", "ctrl+b", "ctrl+f", "shift+up", "shift+down":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.growInput()
	return m, cmd
}

func (m *Model) submit() tea.Cmd {
	prompt := strings.TrimSpace(m.input.Value())
	if prompt == "" {
		return nil
	}
	m.input.Reset()
	m.growInput()

	m.push(entry{kind: entryUser, text: prompt})
	m.history = append(m.history, llm.UserText(prompt))

	m.streaming = true
	m.startedAt = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	history := m.history
	go m.run(ctx, history)

	return m.spinner.Tick
}

func (m *Model) run(ctx context.Context, history []llm.Message) {
	emit := func(msg tea.Msg) {
		select {
		case m.events <- msg:
		case <-ctx.Done():
		}
	}

	m.agent.Hooks = agent.Hooks{
		OnTextDelta:     func(text string) { emit(textDeltaMsg(text)) },
		OnThinkingDelta: func(text string) { emit(thinkDeltaMsg(text)) },
		OnToolCall:      func(call llm.ToolUseBlock) { emit(toolCallMsg{Call: call}) },
		OnToolResult: func(call llm.ToolUseBlock, result string, isError bool) {
			emit(toolResultMsg{Call: call, Result: result, IsError: isError})
		},
		OnTurn: func(resp *llm.Response) {
			emit(turnMsg{Model: resp.Model, Usage: resp.Usage, Stop: resp.StopReason})
		},
	}

	updated, err := m.agent.Run(ctx, history)
	// Unlike emit, this send ignores ctx: dropping it on cancellation would
	// leave the UI stuck streaming forever.

	m.events <- doneMsg{History: updated, Err: err}
}

func (m *Model) finishTurn(msg doneMsg) {
	m.streaming = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if len(msg.History) > 0 {
		m.history = msg.History
	}

	switch {
	case msg.Err == nil:
	case errors.Is(msg.Err, context.Canceled):
		m.push(entry{kind: entryNotice, text: "interrupted"})
	case errors.Is(msg.Err, agent.ErrTruncated):
		m.push(entry{kind: entryNotice, text: "response truncated — raise --max-tokens"})
	default:
		m.push(entry{kind: entryError, text: msg.Err.Error()})
	}
}

func (m *Model) reset() {
	m.history = nil
	m.entries = nil
	m.rendered = nil
	m.sessionUsage = llm.Usage{}
	m.push(entry{kind: entryNotice, text: "context cleared"})
}

func (m *Model) push(e entry) {
	m.entries = append(m.entries, e)
	m.rendered = append(m.rendered, e.render(m.contentWidth()))
	m.syncViewport()
}

func (m *Model) appendToOpenEntry(kind entryKind, text string) {
	if n := len(m.entries); n > 0 && m.entries[n-1].kind == kind {
		m.entries[n-1].text += text
		m.rendered[n-1] = m.entries[n-1].render(m.contentWidth())
		m.syncViewport()
		return
	}
	m.push(entry{kind: kind, text: text})
}

func (m *Model) completeTool(msg toolResultMsg) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.kind != entryTool || e.done || e.toolName != msg.Call.Name {
			continue
		}
		e.done = true
		e.failed = msg.IsError
		e.resultLen = len(msg.Result)
		m.entries[i] = e
		m.rendered[i] = e.render(m.contentWidth())
		m.syncViewport()
		return
	}
}

func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.transcript())
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) transcript() string {
	parts := make([]string, 0, len(m.rendered))
	for _, r := range m.rendered {
		if r != "" {
			parts = append(parts, r)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (m *Model) rerender() {
	width := m.contentWidth()
	m.rendered = m.rendered[:0]
	for _, e := range m.entries {
		m.rendered = append(m.rendered, e.render(width))
	}
}

func (m *Model) contentWidth() int { return max(m.width-2, 20) }

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height

	m.input.SetWidth(max(m.contentWidth()-gutterWidth, 20))

	if !m.ready {
		m.viewport = viewport.New(m.contentWidth(), m.viewportHeight())
		m.ready = true
	} else {
		m.viewport.Width = m.contentWidth()
		m.viewport.Height = m.viewportHeight()
	}

	m.rerender()
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.transcript())
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) viewportHeight() int {
	chrome := 1 + 1 + 1 + m.input.Height() + 1
	return max(m.height-chrome, 3)
}

func (m *Model) growInput() {
	want := min(max(m.input.LineCount(), 1), maxInputHeight)
	if want == m.input.Height() {
		return
	}
	m.input.SetHeight(want)
	if m.ready {
		m.viewport.Height = m.viewportHeight()
		m.syncViewport()
	}
}

func (m *Model) View() string {
	if !m.ready {
		return "\n  starting zaino…\n"
	}
	if m.quitting {
		return ""
	}

	pad := lipgloss.NewStyle().PaddingLeft(1)
	return strings.Join([]string{
		pad.Render(m.header()),
		"",
		pad.Render(m.viewport.View()),
		pad.Render(rule(m.contentWidth())),
		pad.Render(m.inputView()),
		pad.Render(m.footer()),
	}, "\n")
}

func (m *Model) header() string {
	left := brandStyle.Render("▚ zaino")

	model := m.lastModel
	if model == "" {
		model = m.agent.Model
	}
	if model == "" && m.agent.Provider != nil {
		model = m.agent.Provider.DefaultModel()
	}
	right := metaStyle.Render(m.provider + " · " + model)
	if m.agent.Effort != "" {
		right = metaStyle.Render(m.provider + " · " + model + " · " + m.agent.Effort)
	}

	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) inputView() string {
	marker := userMarker.Render("›")
	if m.streaming {
		marker = m.spinner.View()
	}
	return marker + strings.Repeat(" ", gutterWidth-lipgloss.Width(marker)) + m.input.View()
}

func (m *Model) footer() string {
	var left string
	if m.streaming {
		left = hintStyle.Render(fmt.Sprintf("working %s · ⌃c stop",
			time.Since(m.startedAt).Round(time.Second)))
	} else {
		left = hintStyle.Render("⏎ send · ⌃j newline · ⌃l clear · ⌃d quit")
	}

	right := metaStyle.Render(m.usageLine())
	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) usageLine() string {
	u := m.sessionUsage
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return ""
	}
	parts := []string{
		humanTokens(u.InputTokens) + "↑",
		humanTokens(u.OutputTokens) + "↓",
	}
	if u.ThinkingTokens > 0 {
		parts = append(parts, humanTokens(u.ThinkingTokens)+"⋯")
	}
	if u.CacheReadTokens > 0 {
		parts = append(parts, humanTokens(u.CacheReadTokens)+"⚡")
	}
	return strings.Join(parts, " ")
}
