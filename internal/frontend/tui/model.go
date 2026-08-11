package tui

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
	"github.com/zenodea/zaino/internal/store/recall"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/wirelog"
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
		Messages []llm.Message
		Err      error
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
	menu     menu
	picker   picker

	recall     *recall.List
	repo       session.Repo
	rec        *session.Recorder
	wire       *wirelog.Log
	saveFailed bool

	entries  []entry
	rendered []string
	messages []llm.Message

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
		rec:      session.NewRecorder(nil),
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
		m.rec.Turn(msg.Usage)
		m.wire.Turn(msg.Model, msg.Stop, msg.Usage)
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
	m.syncInputChrome()
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picker.open {
		return m, m.handlePickerKey(msg)
	}

	if m.menu.open {
		switch msg.String() {
		case "up", "ctrl+p":
			m.moveMenu(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveMenu(1)
			return m, nil
		case "tab":
			m.complete()
			return m, nil
		case "enter":
			// Run what is highlighted, so "/cl⏎" is "/clear".
			if c, ok := m.selected(); ok {
				m.input.Reset()
				m.syncInputChrome()
				return m, c.run(m, "")
			}
		case "esc":
			m.menu = menu{}
			return m, nil
		}
	}

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
		m.syncInputChrome()
		return m, nil

	case "ctrl+l":
		m.reset()
		return m, nil

	case "pgup", "pgdown", "ctrl+u", "ctrl+b", "ctrl+f", "shift+up", "shift+down":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "up", "ctrl+p":
		if m.recallPrev() {
			return m, nil
		}

	case "down", "ctrl+n":
		if m.recallNext() {
			return m, nil
		}
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	// Editing by hand ends the browse: the box is yours again.
	if m.recall != nil && m.input.Value() != before {
		m.recall.Reset()
	}
	m.syncInputChrome()
	return m, cmd
}

// Already browsing keeps browsing, wherever the cursor sits: otherwise a
// recalled multi-line prompt drops you out of recall half way through.
func (m *Model) recallPrev() bool {
	if m.recall == nil {
		return false
	}
	if m.input.Line() != 0 || m.input.LineInfo().RowOffset != 0 {
		return false
	}
	if !m.recall.Browsing() &&
		strings.TrimSpace(m.input.Value()) != "" &&
		m.input.LineInfo().ColumnOffset != 0 {
		return false
	}

	line, ok := m.recall.Prev(m.input.Value())
	if !ok {
		return false
	}
	m.showRecalled(line)
	return true
}

func (m *Model) recallNext() bool {
	if m.recall == nil || !m.recall.Browsing() {
		return false
	}
	if m.input.Line() != m.input.LineCount()-1 {
		return false
	}

	line, ok := m.recall.Next()
	if !ok {
		return false
	}
	m.showRecalled(line)
	return true
}

func (m *Model) showRecalled(line string) {
	m.input.SetValue(line)
	m.input.CursorEnd()
	m.syncInputChrome()
}

func (m *Model) submit() tea.Cmd {
	prompt := strings.TrimSpace(m.input.Value())
	if prompt == "" {
		return nil
	}
	m.input.Reset()
	m.syncInputChrome()

	if isCommandLine(prompt) {
		return m.runCommand(prompt)
	}

	if m.recall != nil {
		m.recall.Add(prompt)
	}

	m.push(entry{kind: entryUser, text: prompt})
	m.messages = append(m.messages, llm.UserText(prompt))

	m.streaming = true
	m.startedAt = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Bound here, not in the goroutine: the agent is shared.
	m.agent.Hooks = m.hooks(ctx)

	messages := m.messages
	go m.run(ctx, messages)

	return m.spinner.Tick
}

func (m *Model) hooks(ctx context.Context) agent.Hooks {
	emit := func(msg tea.Msg) {
		select {
		case m.events <- msg:
		case <-ctx.Done():
		}
	}

	return agent.Hooks{
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
}

func (m *Model) run(ctx context.Context, messages []llm.Message) {
	updated, err := m.agent.Run(ctx, messages)
	// Unlike emit, this send ignores ctx: dropping it on cancellation would
	// leave the UI stuck streaming forever.

	m.events <- doneMsg{Messages: updated, Err: err}
}

func (m *Model) finishTurn(msg doneMsg) {
	m.streaming = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if len(msg.Messages) > 0 {
		m.messages = msg.Messages
	}
	m.saveError(m.rec.Messages(m.messages))

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
	m.clearContext()
	m.push(entry{kind: entryNotice, text: "context cleared"})
}

func (m *Model) clearContext() {
	m.saveError(m.rec.Clear())
	m.messages = nil
	m.entries = nil
	m.rendered = nil
	m.sessionUsage = llm.Usage{}
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
	chrome := 1 + 1 + 1 + m.input.Height() + m.menuHeight() + 1
	return max(m.height-chrome, 3)
}

func (m *Model) syncInputChrome() {
	if want := min(max(m.input.LineCount(), 1), maxInputHeight); want != m.input.Height() {
		m.input.SetHeight(want)
	}
	m.refreshMenu()

	if !m.ready {
		return
	}
	if height := m.viewportHeight(); height != m.viewport.Height {
		m.viewport.Height = height
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

	body := m.viewport.View()
	if m.picker.open {
		body = m.pickerView()
	}

	lines := []string{
		pad.Render(m.header()),
		"",
		pad.Render(body),
	}
	if panel := m.menuView(); panel != "" {
		lines = append(lines, "", pad.Render(panel))
	}
	lines = append(lines,
		pad.Render(rule(m.contentWidth())),
		pad.Render(m.inputView()),
		pad.Render(m.footer()),
	)
	return strings.Join(lines, "\n")
}

func (m *Model) header() string {
	left := brandStyle.Render("▚ zaino")

	right := metaStyle.Render(m.provider + " · " + m.modelName())
	if m.agent.Effort != "" {
		right = metaStyle.Render(m.provider + " · " + m.modelName() + " · " + m.agent.Effort)
	}

	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clamp(left, m.contentWidth())
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
	roomy := m.contentWidth() >= 64

	var hint string
	switch {
	case m.streaming:
		hint = fmt.Sprintf("working %s · ⌃c stop",
			time.Since(m.startedAt).Round(time.Second))
	case m.menu.open && roomy:
		hint = "⇥ complete · ⏎ run · ↑↓ choose · esc dismiss"
	case m.menu.open:
		hint = "⇥ complete · ⏎ run · esc dismiss"
	case roomy:
		hint = "⏎ send · ⌃j newline · / commands · ⌃l clear · ⌃d quit"
	default:
		hint = "⏎ send · / commands · ⌃d quit"
	}
	left := hintStyle.Render(hint)

	right := metaStyle.Render(m.usageLine())
	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clamp(left, m.contentWidth())
	}
	return left + strings.Repeat(" ", gap) + right
}

func clamp(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
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
