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
	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/store/credentials"
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
	compactMsg struct {
		Summary string
		Kept    []llm.Message
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
	journey  journey
	chooser  chooser
	sheet    sheet
	secret   secret

	cfg    *config.Config
	custom []command

	creds      *credentials.Store
	recall     *recall.List
	repo       session.Repo
	rec        *session.Recorder
	wire       *wirelog.Log
	saveFailed bool

	entries  []entry
	rendered []string
	messages []llm.Message

	tasks     []*task
	taskIndex map[string]*task
	agents    agentsBoard

	events chan tea.Msg
	cancel context.CancelFunc

	width, height int
	ready         bool

	streaming    bool
	startedAt    time.Time
	sessionUsage llm.Usage
	lastModel    string

	pending *pendingAsk
	limit   limitGate
	vim     vim

	cursor  int
	tops    []int
	heights []int
	motion  motion

	quitting  bool
	quitArmed bool

	// Set by an overlay's apply, which cannot return a command itself.
	queued tea.Cmd

	fetched        map[string][]string
	awaitingModels bool
}

func (m *Model) runCmd(cmd tea.Cmd) { m.queued = cmd }

func (m *Model) takeQueued() tea.Cmd {
	cmd := m.queued
	m.queued = nil
	return cmd
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
		agent:     ag,
		provider:  providerName,
		input:     input,
		spinner:   sp,
		rec:       session.NewRecorder(nil),
		events:    make(chan tea.Msg, 64),
		cursor:    -1,
		motion:    motion{barAt: -1, barTo: -1},
		taskIndex: map[string]*task{},
		agents:    agentsBoard{viewing: -1},
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick, m.waitForEvent())
}

func (m *Model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return <-m.events }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	return model, m.withFrame(cmd)
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			toolID:    msg.Call.ID,
			toolArgs:  compactArgs(msg.Call.Input, argsLimit),
			toolInput: string(msg.Call.Input),
		})
		return m, m.waitForEvent()

	case toolResultMsg:
		m.completeTool(msg)
		return m, m.waitForEvent()

	case taskStartMsg:
		m.startTask(msg)
		return m, m.waitForEvent()

	case taskMsg:
		m.taskEvent(msg.id, msg.msg)
		return m, m.waitForEvent()

	case taskDoneMsg:
		m.finishTask(msg)
		return m, m.waitForEvent()

	case modelsMsg:
		return m, m.receiveModels(msg)

	case loginDoneMsg:
		return m, m.finishLogin(msg)

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

	case compactMsg:
		m.compacted(msg)
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
		if m.journey.open {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.moveJourney(-1)
			case tea.MouseButtonWheelDown:
				m.moveJourney(1)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	if m.secret.open {
		// The composer is not the focused field while a key is being typed,
		// so the blink has to follow the masked one.
		m.secret.input, cmd = m.secret.input.Update(msg)
		return m, cmd
	}
	m.input, cmd = m.input.Update(msg)
	m.syncInputChrome()
	return m, cmd
}

// overlay floats a panel over the last lines of a block. Anything that opens
// while you type belongs on top of the transcript, not wedged into it.
func overlay(base, panel string) string {
	if panel == "" {
		return base
	}
	baseLines := strings.Split(base, "\n")
	panelLines := strings.Split(panel, "\n")

	if len(panelLines) >= len(baseLines) {
		return strings.Join(panelLines[len(panelLines)-len(baseLines):], "\n")
	}
	copy(baseLines[len(baseLines)-len(panelLines):], panelLines)
	return strings.Join(baseLines, "\n")
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		m.quitArmed = false
		return m.handleAskKey(msg)
	}

	if m.limit.open {
		m.quitArmed = false
		return m.handleLimitKey(msg)
	}

	// Ahead of every other overlay: while a key is being typed, no keystroke
	// may reach a binding that could echo or record it.
	if m.secret.open {
		m.quitArmed = false
		return m.handleSecretKey(msg)
	}

	if m.picker.open {
		m.quitArmed = false
		return m, m.handlePickerKey(msg)
	}

	if m.journey.open {
		m.quitArmed = false
		return m, m.handleJourneyKey(msg)
	}

	if m.agents.open {
		m.quitArmed = false
		return m, m.handleAgentsKey(msg)
	}

	if m.sheet.open {
		m.quitArmed = false
		return m.handleSheetKey(msg)
	}

	if m.chooser.open {
		m.quitArmed = false
		return m.handleChooserKey(msg)
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
		case "up", "ctrl+p", "ctrl+k":
			return m, m.moveMenu(-1)
		case "down", "ctrl+n", "ctrl+j":
			return m, m.moveMenu(1)
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
		if m.toggleSelected() {
			return m, nil
		}
		if m.streaming && !m.typedLive() {
			if line := strings.TrimSpace(m.input.Value()); isCommandLine(line) {
				if name, _ := splitCommand(line); m.known(name) {
					m.notice("/%s waits for the turn to finish", name)
				}
			}
			return m, nil
		}
		return m, m.submit()

	// ⌃↑/⌃↓ for terminals where a multiplexer has claimed ⌃j/⌃k; and once
	// the bar is up, plain j/k, since there is nothing else for them to do
	// until you type something.
	case "ctrl+j", "ctrl+down":
		return m, m.moveCursor(1)

	case "ctrl+k", "ctrl+up":
		return m, m.moveCursor(-1)

	case "j", "k":
		if m.cursor >= 0 && strings.TrimSpace(m.input.Value()) == "" {
			if msg.String() == "j" {
				return m, m.moveCursor(1)
			}
			return m, m.moveCursor(-1)
		}

	case "alt+enter", "shift+enter", "ctrl+o":
		m.input.InsertString("\n")
		m.syncInputChrome()
		return m, nil

	case "shift+tab":
		if gate := m.agent.Gate; gate != nil && gate.Policy != nil {
			gate.Policy.SetMode(gate.Mode().Next())
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
	return m.send(prompt, prompt)
}

// Hands the conversation as it stands to the agent. Sending a fresh prompt and
// re-sending one the ceiling stopped differ only in what came before this.
func (m *Model) launch() tea.Cmd {
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
		OnCompact: func(summary string, kept []llm.Message) {
			emit(compactMsg{Summary: summary, Kept: kept})
		},
		OnTask: func(info agent.TaskInfo) agent.Hooks {
			emit(taskStartMsg{info: info})
			return m.taskHooks(emit, info)
		},
		OnTaskDone: func(id string, history []llm.Message, err error) {
			emit(taskDoneMsg{id: id, history: history, failed: err != nil})
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

	var overLimit *agent.ContextLimitError

	switch {
	case msg.Err == nil:
	case errors.As(msg.Err, &overLimit):
		m.holdAtLimit(overLimit)
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
	m.tasks = nil
	m.taskIndex = map[string]*task{}
	m.agents = agentsBoard{viewing: -1}
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
	at := completeEntry(m.entries, msg)
	if at < 0 {
		return
	}
	m.rendered[at] = m.entries[at].render(m.contentWidth())
	m.syncViewport()
}

// Matched by tool-use ID: several calls of the same tool can be in flight.
func completeEntry(entries []entry, msg toolResultMsg) int {
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.kind != entryTool || e.done {
			continue
		}
		if e.toolID != msg.Call.ID && !(e.toolID == "" && e.toolName == msg.Call.Name) {
			continue
		}
		entries[i].done = true
		entries[i].failed = msg.IsError
		entries[i].resultLen = len(msg.Result)
		entries[i].toolResult = msg.Result
		return i
	}
	return -1
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
	dot := metaStyle.Render("  ·  ")
	keys := []string{
		strings.Join([]string{
			keyHint("⏎", "send"), keyHint("/", "commands"),
			keyHint("⇧⇥", "permission mode"), keyHint("esc", "normal mode"),
		}, dot),
		strings.Join([]string{
			keyHint("⏎", "send"), keyHint("/", "commands"), keyHint("⇧⇥", "permission"),
		}, dot),
		strings.Join([]string{keyHint("⏎", "send"), keyHint("/", "commands")}, dot),
	}

	lines := append(strings.Split(logo(), "\n"),
		"",
		hintStyle.Render("an agent harness, in a backpack"),
		"",
		fit(keys, m.contentWidth()),
	)

	// Centred on both axes: an empty screen has nothing else to hang it on.
	for i, line := range lines {
		lines[i] = indent(line, (m.contentWidth()-lipgloss.Width(line))/2)
	}
	if top := (m.viewport.Height - len(lines)) / 2; top > 0 {
		lines = append(make([]string, top), lines...)
	}
	return strings.Join(lines, "\n")
}

func indent(s string, by int) string {
	if by <= 0 {
		return s
	}
	return strings.Repeat(" ", by) + s
}

func (m *Model) transcript() string {
	if len(m.entries) == 0 {
		return m.splash()
	}

	var out []string
	previous := entryNotice

	// Where each entry lands is recorded as the transcript is built, in lines:
	// an entry is one element of out but as many lines as it wraps to, and the
	// bar, the trail and the scroll all count lines.
	m.tops = make([]int, len(m.entries))
	m.heights = make([]int, len(m.entries))
	line := 0

	for i, rendered := range m.rendered {
		if rendered == "" {
			continue
		}
		kind := m.entries[i].kind

		if len(out) > 0 && !(tight(previous) && tight(kind)) {
			out = append(out, m.blankRow(line))
			line++
		}

		m.tops[i] = line
		height := strings.Count(rendered, "\n") + 1
		m.heights[i] = height

		// Bars do not change how anything wraps — they sit in the gutter that
		// is already there — so the line count is the same either way.
		if bars := m.barsWithin(line, height); len(bars) > 0 {
			rendered = m.entries[i].renderAs(m.contentWidth(), bars)
		}
		out = append(out, rendered)
		line += height
		previous = kind
	}
	return strings.Join(out, "\n")
}

func (m *Model) barsWithin(top, height int) map[int]string {
	var bars map[int]string
	for line := top; line < top+height; line++ {
		if bar := m.barForLine(line); bar != noBar() {
			if bars == nil {
				bars = map[int]string{}
			}
			bars[line-top] = bar
		}
	}
	return bars
}

// The gap between two entries is ground the bar can be on, so it is a row like
// any other rather than an empty string.
func (m *Model) blankRow(line int) string {
	if bar := m.barForLine(line); bar != noBar() {
		return " " + bar
	}
	return ""
}

func tight(kind entryKind) bool {
	return kind == entryThinking || kind == entryTool
}

func (m *Model) rerender() {
	width := m.contentWidth()
	m.rendered = m.rendered[:0]
	for _, e := range m.entries {
		m.rendered = append(m.rendered, e.render(width))
	}
	for _, t := range m.tasks {
		t.rendered = t.rendered[:0]
		for _, e := range t.entries {
			t.rendered = append(t.rendered, e.render(width))
		}
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
	// The menu is not in this sum: it floats over the transcript rather than
	// pushing it, so opening it must not resize anything underneath.
	chrome := 1 + 1 + 1 + 1 + 1 + m.input.Height() + m.chooserHeight() + 1
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

	if m.secret.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			m.secretView(),
		}, "\n")
	}

	if m.picker.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			pad.Render(m.pickerView()),
			pad.Render(rule(m.contentWidth())),
			pad.Render(m.pickerFooter()),
		}, "\n")
	}

	if m.journey.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			pad.Render(m.journeyView()),
			pad.Render(rule(m.contentWidth())),
			pad.Render(m.journeyFooter()),
		}, "\n")
	}

	if m.agents.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			pad.Render(m.agentsView()),
			pad.Render(rule(m.contentWidth())),
			pad.Render(m.agentsFooter()),
		}, "\n")
	}

	if m.sheet.open {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			m.sheetView(),
			pad.Render(rule(m.contentWidth())),
			pad.Render(m.sheetFooter()),
		}, "\n")
	}

	// A board is a screen of its own, for the same reason the picker is.
	if m.onBoard() {
		return strings.Join([]string{
			pad.Render(m.header()),
			"",
			m.boardView(),
			pad.Render(rule(m.contentWidth())),
			pad.Render(hintStyle.Render("j/k or ↑↓ choose · ⏎ set · esc cancel")),
		}, "\n")
	}

	// The blank under the transcript is always there: the last thing said never
	// sits on the rule, panel or no panel. The menu floats over the gap too, so
	// it comes down onto the rule as before.
	ground := pad.Render(m.viewport.View()) + "\n"

	var below []string
	switch {
	case m.limitView() != "":
		below = []string{pad.Render(m.limitView())}
	case m.askView() != "":
		below = []string{pad.Render(m.askView())}
	case m.chooserView() != "":
		below = []string{pad.Render(m.chooserView())}
	case m.menuView() != "":
		ground = overlay(ground, pad.Render(m.menuView()))
	}

	lines := append([]string{pad.Render(m.header()), "", ground}, below...)
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

	case m.streaming && m.runningTasks() > 0:
		return []string{fmt.Sprintf("working %s · %s out · /agents watches them · ⌃c stop",
			time.Since(m.startedAt).Round(time.Second), plural(m.runningTasks(), "agent")),
			fmt.Sprintf("%s out · /agents · ⌃c stop", plural(m.runningTasks(), "agent")),
			"⌃c stop"}

	case m.streaming:
		return []string{fmt.Sprintf("working %s · ⌃c stop",
			time.Since(m.startedAt).Round(time.Second)), "⌃c stop"}

	case m.chooser.open && m.chooser.layout == layoutScale:
		return []string{
			"←→ or h/l turn it up and down · ⏎ set · esc cancel",
			"←→ choose · ⏎ set · esc",
			"⏎ set · esc",
		}

	case m.chooser.open:
		return []string{
			"↑↓ or j/k choose · ⏎ pick · esc cancel",
			"↑↓ choose · ⏎ pick · esc",
			"⏎ pick · esc",
		}

	case m.menu.open:
		return []string{
			"⇥ complete · ⏎ run · ↑↓ or ⌃j ⌃k choose · esc dismiss",
			"⇥ complete · ⏎ run · esc dismiss",
			"⏎ run · esc",
		}

	case m.cursor >= 0:
		if e, ok := m.selectedEntry(); ok && e.kind == entryTool {
			if _, spawned := m.taskIndex[e.toolID]; spawned {
				return []string{
					"⏎ walk into the agent · j/k move · type to go back to the composer",
					"⏎ walk in · j/k move",
					"⏎ walk in",
				}
			}
		}
		return []string{
			"⏎ expand · j/k move · type to go back to the composer",
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
	if n := m.runningTasks(); n > 0 {
		chips = append(chips, agentChip.Render("⚒ "+plural(n, "agent")))
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
