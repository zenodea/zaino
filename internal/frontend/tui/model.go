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
	"github.com/zenodea/zaino/internal/permission"
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

	pending *pendingAsk
	vim     vim

	cursor  int
	tops    []int
	heights []int
	motion  motion

	quitting  bool
	quitArmed bool
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
		cursor:   -1,
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
			kind:      entryTool,
			toolName:  msg.Call.Name,
			toolArgs:  compactArgs(msg.Call.Input, argsLimit),
			toolInput: string(msg.Call.Input),
		})
		return m, m.waitForEvent()

	case toolResultMsg:
		m.completeTool(msg)
		return m, m.waitForEvent()

	case askMsg:
		m.pending = &pendingAsk{req: msg.req, reply: msg.reply}
		m.syncViewport()
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

	case frameMsg:
		return m, m.step()

	case tea.MouseMsg:
		if m.picker.open {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.movePicker(-1)
			case tea.MouseButtonWheelDown:
				m.movePicker(1)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncInputChrome()
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		m.quitArmed = false
		return m.handleAskKey(msg)
	}

	if m.picker.open {
		m.quitArmed = false
		return m, m.handlePickerKey(msg)
	}

	if msg.String() == "esc" && !m.vimEnabled() && m.streaming && m.cancel != nil {
		m.cancel()
		return m, nil
	}

	if m.inNormalMode() && !m.menu.open {
		return m.handleNormalKey(msg)
	}
	if m.vimEnabled() && msg.String() == "esc" && !m.menu.open {
		m.enterNormal()
		return m, nil
	}

	return m.handleAppKey(msg)
}

func (m *Model) handleAppKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {

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
		}
	}

	armed := m.quitArmed
	m.quitArmed = false

	switch msg.String() {
	case "ctrl+c":
		if m.streaming && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		if armed {
			m.quitting = true
			return m, tea.Quit
		}
		m.quitArmed = true
		return m, nil

	case "enter":
		if m.streaming {
			return m, nil
		}
		if m.toggleSelected() {
			return m, nil
		}
		return m, m.submit()

	case "ctrl+j":
		return m, m.moveCursor(1)

	case "ctrl+k":
		return m, m.moveCursor(-1)

	case "alt+enter", "shift+enter", "ctrl+o":
		m.input.InsertString("\n")
		m.syncInputChrome()
		return m, nil

	case "shift+tab":
		if gate := m.agent.Gate; gate != nil && gate.Policy != nil {
			mode := gate.Mode().Next()
			gate.Policy.SetMode(mode)
			m.notice("permission → %s", mode)
		}
		return m, nil

	case "pgup", "pgdown", "ctrl+u", "ctrl+d", "ctrl+b", "ctrl+f", "shift+up", "shift+down":
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
	if m.input.Value() != before {
		m.clearCursor()
		if m.recall != nil {
			m.recall.Reset()
		}
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
	m.cursor = -1
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
		m.rendered[n-1] = m.entries[n-1].renderAs(m.contentWidth(), m.barFor(n-1))
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
		e.toolResult = msg.Result
		m.entries[i] = e
		m.rendered[i] = e.renderAs(m.contentWidth(), m.barFor(i))
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

func (m *Model) splash() string {
	keys := []string{
		keyHint("⏎", "send"),
		keyHint("/", "commands"),
		keyHint("⇧⇥", "permission mode"),
		keyHint("esc", "normal mode"),
	}
	lines := append(strings.Split(logo(), "\n"),
		"",
		hintStyle.Render("an agent harness, in a backpack"),
		"",
		strings.Join(keys, metaStyle.Render("  ·  ")),
	)
	return lipgloss.NewStyle().PaddingTop(1).Render(strings.Join(lines, "\n"))
}

func (m *Model) transcript() string {
	if len(m.entries) == 0 {
		return m.splash()
	}

	var out []string
	previous := entryNotice

	// Where each entry lands is recorded as the transcript is built. Counting it
	// again afterwards means counting the blank lines between entries too.
	m.tops = make([]int, len(m.entries))
	m.heights = make([]int, len(m.entries))

	for i, rendered := range m.rendered {
		if rendered == "" {
			continue
		}
		kind := m.entries[i].kind

		if len(out) > 0 && !(tight(previous) && tight(kind)) {
			out = append(out, "")
		}

		m.tops[i] = len(out)
		m.heights[i] = strings.Count(rendered, "\n") + 1
		out = append(out, rendered)
		previous = kind
	}
	return strings.Join(out, "\n")
}

func tight(kind entryKind) bool {
	return kind == entryThinking || kind == entryTool
}

func (m *Model) rerender() {
	width := m.contentWidth()
	m.rendered = m.rendered[:0]
	for i, e := range m.entries {
		m.rendered = append(m.rendered, e.renderAs(width, m.barFor(i)))
	}
	m.syncViewport()
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
	chrome := 1 + 1 + 1 + 1 + m.input.Height() + m.menuHeight() + 1
	return max(m.height-chrome, 3)
}

func (m *Model) syncInputChrome() {
	if want := min(max(m.input.LineCount(), 1), maxInputHeight); want != m.input.Height() {
		m.input.SetHeight(want)
	}
	m.refreshMenu()
	m.syncHeight()
}

// The panel takes room from the transcript, so the viewport has to be resized
// in the same breath as the panel opening or closing.
func (m *Model) syncHeight() {
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

	if m.picker.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			pad.Render(m.pickerView()),
			pad.Render(rule(m.contentWidth())),
			pad.Render(m.pickerFooter()),
		}, "\n")
	}

	lines := []string{
		pad.Render(m.header()),
		"",
		pad.Render(m.viewport.View()),
	}
	if panel := m.askView(); panel != "" {
		lines = append(lines, "", pad.Render(panel))
	} else if panel := m.menuView(); panel != "" {
		lines = append(lines, "", pad.Render(panel))
	}
	lines = append(lines,
		pad.Render(rule(m.contentWidth())),
		pad.Render(m.inputView()),
		"",
		pad.Render(m.footer()),
	)
	return strings.Join(lines, "\n")
}

func (m *Model) header() string {
	left := brandMark() + " " + brandStyle.Render("zaino")

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

	body := m.input.View()
	if m.inVisualMode() {
		body = m.visualView()
	}

	pad := strings.Repeat(" ", gutterWidth-lipgloss.Width(marker))
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = marker + pad + line
			continue
		}
		lines[i] = strings.Repeat(" ", gutterWidth) + line
	}
	return strings.Join(lines, "\n")
}

func (m *Model) pickerFooter() string {
	return hintStyle.Render("j/k or ↑↓ move · ⏎ resume · g/G ends · q back")
}

func (m *Model) footer() string {
	status := m.status()
	right := metaStyle.Render(m.usageLine())

	room := m.contentWidth() - lipgloss.Width(status) - lipgloss.Width(right) - 1
	left := status + hintStyle.Render(fit(m.hints(), room))

	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clamp(left, m.contentWidth())
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) hints() []string {
	switch {
	case m.quitArmed:
		return []string{
			"⌃c again to quit · /exit also works · any other key to stay",
			"⌃c again to quit · any key to stay",
			"⌃c again to quit",
		}

	case m.pending != nil:
		return []string{"waiting on you · y allow · a session · n refuse", "y · a · n"}

	case m.streaming:
		return []string{fmt.Sprintf("working %s · ⌃c stop",
			time.Since(m.startedAt).Round(time.Second)), "⌃c stop"}

	case m.menu.open:
		return []string{
			"⇥ complete · ⏎ run · ↑↓ choose · esc dismiss",
			"⇥ complete · ⏎ run · esc dismiss",
			"⏎ run · esc",
		}

	case m.cursor >= 0:
		return []string{
			"⏎ expand · ⌃j⌃k move · type to go back to the composer",
			"⏎ expand · ⌃j⌃k move",
			"⏎ expand",
		}

	case m.inVisualMode():
		return []string{
			"hjkl w b e extend · d delete · c change · y yank · esc cancel",
			"hjkl extend · d delete · c change · y yank · esc",
			"d delete · c change · y yank · esc",
		}

	case m.inNormalMode():
		return []string{
			"⏎ send · i insert · v visual · hjkl move · dd cc x u edit",
			"⏎ send · i insert · v visual · hjkl move",
			"⏎ send · i insert · v visual",
			"⏎ send · i insert",
		}
	}
	return []string{
		"⏎ send · ⌃j⌃k chat · ⌥⏎ newline · / commands",
		"⏎ send · ⌃j⌃k chat · / commands",
		"⏎ send · / commands",
	}
}

func fit(candidates []string, room int) string {
	for _, candidate := range candidates {
		if lipgloss.Width(candidate) <= room {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return clamp(candidates[len(candidates)-1], max(room, 0))
}

func (m *Model) status() string {
	if m.quitArmed {
		return quitChip.Render("⏻ quit?") + metaStyle.Render("  ")
	}

	var chips []string

	if mode := m.agent.Gate.Mode(); m.agent.Gate != nil {
		chips = append(chips, permissionChip(mode))
	}
	if m.vimEnabled() {
		style := hintStyle
		if m.vim.mode == modeNormal {
			style = vimNormalChip
		}
		if m.inVisualMode() {
			style = vimVisualChip
		}
		chips = append(chips, style.Render(m.modeName()))
	}
	if len(chips) == 0 {
		return ""
	}
	return strings.Join(chips, metaStyle.Render(" ")) + metaStyle.Render("  ")
}

func permissionChip(mode permission.Mode) string {
	switch mode {
	case permission.AcceptEdits:
		return acceptChip.Render("⏵⏵ accept edits")
	case permission.Plan:
		return planChip.Render("◇ plan")
	case permission.Bypass:
		return bypassChip.Render("⏵⏵ bypass")
	}
	return manualChip.Render("⏸ manual")
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
