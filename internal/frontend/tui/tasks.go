package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

type (
	taskStartMsg struct{ info agent.TaskInfo }

	taskMsg struct {
		id  string
		msg tea.Msg
	}

	taskDoneMsg struct {
		id      string
		history []llm.Message
		failed  bool
	}
)

type task struct {
	id    string
	what  string
	agent string
	model string
	depth int

	started    time.Time
	ended      time.Time
	done       bool
	failed     bool
	background bool
	cancel     context.CancelFunc

	entries  []entry
	rendered []string

	calls    int
	turns    int
	usage    llm.Usage
	lastCall string
}

func (t *task) note() string {
	var parts []string
	if t.agent != "" {
		parts = append(parts, t.agent)
	}
	if t.background {
		parts = append(parts, "background")
	}
	if t.done {
		parts = append(parts, plural(t.calls, "tool"))
		if t.usage.OutputTokens > 0 {
			parts = append(parts, humanTokens(t.usage.OutputTokens)+"↓")
		}
		return strings.Join(parts, " · ")
	}

	if t.lastCall != "" {
		parts = append(parts, truncate(t.lastCall, 48))
	}
	if t.calls > 0 {
		parts = append(parts, plural(t.calls, "tool"))
	}
	if len(parts) == 0 {
		return "working…"
	}
	return strings.Join(parts, " · ")
}

func plural(n int, what string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", what)
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func (t *task) elapsed() time.Duration {
	if t.done {
		return t.ended.Sub(t.started)
	}
	return time.Since(t.started)
}

func (m *Model) taskHooks(emit func(tea.Msg), info agent.TaskInfo) agent.Hooks {
	wrap := func(msg tea.Msg) { emit(taskMsg{id: info.ID, msg: msg}) }

	return agent.Hooks{
		OnTextDelta:     func(text string) { wrap(textDeltaMsg(text)) },
		OnThinkingDelta: func(text string) { wrap(thinkDeltaMsg(text)) },
		OnToolCall:      func(call llm.ToolUseBlock) { wrap(toolCallMsg{Call: call}) },
		OnToolResult: func(call llm.ToolUseBlock, result string, isError bool) {
			wrap(toolResultMsg{Call: call, Result: result, IsError: isError})
		},
		OnToolProgress: func(call llm.ToolUseBlock, chunk string) {
			wrap(toolProgressMsg{Call: call, Chunk: chunk})
		},
		OnTurn: func(resp *llm.Response) {
			wrap(turnMsg{Model: resp.Model, Usage: resp.Usage, Stop: resp.StopReason})
		},
		OnTask: func(child agent.TaskInfo) agent.Hooks {
			emit(taskStartMsg{info: child})
			return m.taskHooks(emit, child)
		},
		OnTaskDone: func(id string, history []llm.Message, err error) {
			emit(taskDoneMsg{id: id, history: history, failed: err != nil})
		},
	}
}

func (m *Model) startTask(msg taskStartMsg) {
	t := &task{
		id:         msg.info.ID,
		what:       msg.info.Description,
		agent:      msg.info.Agent,
		model:      msg.info.Model,
		depth:      msg.info.Depth,
		started:    time.Now(),
		background: msg.info.Background,
		cancel:     msg.info.Cancel,
	}
	m.tasks = append(m.tasks, t)
	m.taskIndex[t.id] = t
	m.updateTaskCard(t)
}

func (m *Model) taskEvent(id string, msg tea.Msg) {
	t, ok := m.taskIndex[id]
	if !ok {
		return
	}

	switch msg := msg.(type) {
	case textDeltaMsg:
		m.appendToTask(t, entryAssistant, string(msg))

	case thinkDeltaMsg:
		m.appendToTask(t, entryThinking, string(msg))

	case toolCallMsg:
		e := entry{
			kind:      entryTool,
			toolName:  msg.Call.Name,
			toolID:    msg.Call.ID,
			toolArgs:  compactArgs(msg.Call.Input, argsLimit),
			toolInput: string(msg.Call.Input),
		}
		m.pushToTask(t, e)
		t.calls++
		t.lastCall = strings.TrimSpace(e.toolName + " " + e.toolSummary())

	case toolProgressMsg:
		if at := progressEntry(t.entries, msg); at >= 0 {
			t.rendered[at] = t.entries[at].render(m.contentWidth())
		}

	case toolResultMsg:
		if at := completeEntry(t.entries, msg); at >= 0 {
			t.rendered[at] = t.entries[at].render(m.contentWidth())
		}

	case turnMsg:
		t.turns++
		addUsage(&t.usage, msg.Usage)
		addUsage(&m.sessionUsage, msg.Usage)
		m.charge(msg.Model, msg.Usage)
		m.wire.Turn(msg.Model, msg.Stop, msg.Usage)
	}
	m.updateTaskCard(t)
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.ThinkingTokens += u.ThinkingTokens
	total.CacheReadTokens += u.CacheReadTokens
	total.CacheWriteTokens += u.CacheWriteTokens
}

func (m *Model) finishTask(msg taskDoneMsg) {
	t, ok := m.taskIndex[msg.id]
	if !ok {
		return
	}
	t.done = true
	t.failed = msg.failed
	t.ended = time.Now()
	m.updateTaskCard(t)

	m.record(session.Task(session.TaskBody{
		ID:          t.id,
		Description: t.what,
		Agent:       t.agent,
		Model:       t.model,
		Depth:       t.depth,
		Failed:      t.failed,
		Messages:    msg.history,
		Usage:       t.usage,
	}))

	if t.background {
		m.deliverReport(t)
	}
}

// A background child's report is already with the agent, via Steer. Mid-turn
// it rides in with the next tool results; between turns it is the next one.
func (m *Model) deliverReport(t *task) {
	m.push(entry{kind: entryNotice, text: fmt.Sprintf("%s reported back", t.what)})
	if m.streaming {
		m.steered++
		return
	}
	left := m.agent.Steered()
	if len(left) == 0 {
		return
	}
	m.messages = append(m.messages, left...)
	m.runCmd(m.launch())
}

func (m *Model) appendToTask(t *task, kind entryKind, text string) {
	if n := len(t.entries); n > 0 && t.entries[n-1].kind == kind {
		t.entries[n-1].text += text
		t.rendered[n-1] = t.entries[n-1].render(m.contentWidth())
		return
	}
	m.pushToTask(t, entry{kind: kind, text: text})
}

func (m *Model) pushToTask(t *task, e entry) {
	t.entries = append(t.entries, e)
	t.rendered = append(t.rendered, e.render(m.contentWidth()))
}

func (m *Model) updateTaskCard(t *task) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind != entryTool || m.entries[i].toolID != t.id {
			continue
		}
		m.entries[i].taskNote = t.note()
		m.rendered[i] = m.entries[i].render(m.contentWidth())
		m.syncViewport()
		return
	}
}

func (m *Model) runningTasks() int {
	n := 0
	for _, t := range m.tasks {
		if !t.done {
			n++
		}
	}
	return n
}

func (m *Model) rebuildTasks(records []session.TaskBody) {
	m.tasks, m.taskIndex = nil, map[string]*task{}
	for _, r := range records {
		t := &task{
			id:     r.ID,
			what:   r.Description,
			agent:  r.Agent,
			model:  r.Model,
			depth:  r.Depth,
			done:   true,
			failed: r.Failed,
			usage:  r.Usage,
		}
		t.entries = transcribeMessages(r.Messages)
		for _, e := range t.entries {
			if e.kind == entryTool {
				t.calls++
			}
			t.rendered = append(t.rendered, e.render(m.contentWidth()))
		}
		m.tasks = append(m.tasks, t)
		m.taskIndex[t.id] = t
	}

	for i, e := range m.entries {
		if t, ok := m.taskIndex[e.toolID]; ok && e.kind == entryTool {
			m.entries[i].taskNote = t.note()
		}
	}
}

type agentsBoard struct {
	open    bool
	cursor  int
	viewing int // index into tasks for the transcript view, -1 for the list

	offset int
	follow bool

	barAt int
	barTo int
	trail map[int]int
}

func cmdAgents(m *Model, _ string) tea.Cmd {
	if len(m.tasks) == 0 {
		m.notice("no agents have been spawned yet — the task tool starts one")
		return nil
	}
	m.agents = agentsBoard{open: true, viewing: -1, barAt: -1, barTo: -1}

	m.agents.cursor = len(m.tasks) - 1
	for i, t := range m.tasks {
		if !t.done {
			m.agents.cursor = i
			break
		}
	}
	_, cursor := m.agentsWindow(m.agentsRows())
	m.agents.barAt, m.agents.barTo = cursor, cursor
	m.syncViewport()
	return nil
}

func (m *Model) closeAgents() {
	m.agents = agentsBoard{viewing: -1}
	m.syncViewport()
}

func (m *Model) openTaskView(id string) {
	for i, t := range m.tasks {
		if t.id != id {
			continue
		}
		if !m.agents.open {
			m.agents = agentsBoard{open: true, barAt: -1, barTo: -1}
		}
		m.agents.cursor = i
		m.agents.viewing = i
		m.agents.offset = 0
		m.agents.follow = true
		return
	}
}

func (m *Model) handleAgentsKey(msg tea.KeyMsg) tea.Cmd {
	if m.agents.viewing >= 0 {
		return m.handleTaskViewKey(msg)
	}

	switch msg.String() {
	case "up", "k", "ctrl+p", "shift+tab":
		m.moveAgents(-1)
	case "down", "j", "ctrl+n", "tab":
		m.moveAgents(1)
	case "pgup", "ctrl+u", "ctrl+b":
		m.moveAgents(-m.agentsRows())
	case "pgdown", "ctrl+d", "ctrl+f":
		m.moveAgents(m.agentsRows())
	case "home", "g":
		m.moveAgents(-len(m.tasks))
	case "end", "G":
		m.moveAgents(len(m.tasks))
	case "enter", "l", "o":
		if m.agents.cursor < len(m.tasks) {
			m.openTaskView(m.tasks[m.agents.cursor].id)
		}
	case "x":
		m.stopTask(m.agents.cursor)
	case "esc", "q", "h", "ctrl+c":
		m.closeAgents()
	}
	return nil
}

func (m *Model) handleTaskViewKey(msg tea.KeyMsg) tea.Cmd {
	t := m.viewedTask()
	if t == nil {
		m.agents.viewing = -1
		return nil
	}
	lines := len(m.taskLines(t))
	rows := m.agentsRows()

	switch msg.String() {
	case "up", "k", "ctrl+p":
		m.scrollTaskView(-1, lines, rows)
	case "down", "j", "ctrl+n":
		m.scrollTaskView(1, lines, rows)
	case "pgup", "ctrl+b":
		m.scrollTaskView(-rows, lines, rows)
	case "pgdown", "ctrl+f":
		m.scrollTaskView(rows, lines, rows)
	case "ctrl+u":
		m.scrollTaskView(-rows/2, lines, rows)
	case "ctrl+d":
		m.scrollTaskView(rows/2, lines, rows)
	case "home", "g":
		m.agents.offset, m.agents.follow = 0, false
	case "end", "G":
		m.agents.offset, m.agents.follow = max(lines-rows, 0), true
	case "x":
		m.stopTask(m.agents.viewing)
	case "esc", "q", "h", "ctrl+c":
		m.agents.viewing = -1
	}
	return nil
}

func (m *Model) scrollTaskView(delta, lines, rows int) {
	bottom := max(lines-rows, 0)
	m.agents.offset = min(max(m.agents.offset+delta, 0), bottom)
	m.agents.follow = m.agents.offset == bottom
}

func (m *Model) viewedTask() *task {
	if m.agents.viewing < 0 || m.agents.viewing >= len(m.tasks) {
		return nil
	}
	return m.tasks[m.agents.viewing]
}

func (m *Model) moveAgents(delta int) {
	n := len(m.tasks)
	if n == 0 {
		return
	}
	m.agents.cursor = min(max(m.agents.cursor+delta, 0), n-1)
	m.aimAgentsBar()
	m.syncViewport()
}

func (m *Model) stopTask(at int) {
	if at < 0 || at >= len(m.tasks) {
		return
	}
	if t := m.tasks[at]; !t.done && t.cancel != nil {
		t.cancel()
	}
}

const agentsChrome = 2

func (m *Model) agentsRows() int {
	return max(m.viewport.Height-agentsChrome, 3)
}

func (m *Model) agentsView() string {
	if t := m.viewedTask(); t != nil {
		return m.taskView(t)
	}

	rows := m.agentsRows()
	window, cursor := m.agentsWindow(rows)

	lines := make([]string, 0, len(window)+2)
	lines = append(lines, m.agentsHeading())
	lines = append(lines, "")

	for i, t := range window {
		style := metaStyle
		if i == cursor {
			style = menuPickStyle
		}
		lines = append(lines, m.agentsBar(i)+" "+m.taskRow(t, style))
	}

	if pad := m.viewport.Height - len(lines); pad > 0 {
		lines = append(make([]string, pad), lines...)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) taskRow(t *task, style lipgloss.Style) string {
	status := "✓"
	statusStyle := assistantMarker
	switch {
	case !t.done:
		status = m.spinner.View()
		statusStyle = spinnerStyle
	case t.failed:
		status, statusStyle = "✗", errorMarker
	}

	indent := strings.Repeat("  ", max(t.depth-1, 0))
	facts := []string{plural(t.calls, "tool"), t.elapsed().Round(time.Second).String()}
	if t.usage.OutputTokens > 0 {
		facts = append(facts, humanTokens(t.usage.OutputTokens)+"↓")
	}
	right := metaStyle.Render(strings.Join(facts, " · "))

	left := statusStyle.Render(status) + " " + indent + style.Render(truncate(t.what, 48))
	if t.agent != "" {
		left += metaStyle.Render(" · " + t.agent)
	}

	gap := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		return clamp(left, m.contentWidth()-2)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) taskView(t *task) string {
	rows := m.agentsRows()
	lines := m.taskLines(t)

	bottom := max(len(lines)-rows, 0)
	if m.agents.follow {
		m.agents.offset = bottom
	}
	m.agents.offset = min(m.agents.offset, bottom)

	window := lines[m.agents.offset:min(m.agents.offset+rows, len(lines))]

	out := make([]string, 0, rows+2)
	out = append(out, m.taskHeading(t))
	out = append(out, "")
	out = append(out, window...)

	if pad := m.viewport.Height - len(out); pad > 0 {
		out = append(out, make([]string, pad)...)
	}
	return strings.Join(out, "\n")
}

func (m *Model) taskLines(t *task) []string {
	var out []string
	previous := entryNotice
	for i, rendered := range t.rendered {
		if rendered == "" {
			continue
		}
		kind := t.entries[i].kind
		if len(out) > 0 && !(tight(previous) && tight(kind)) {
			out = append(out, "")
		}
		out = append(out, strings.Split(rendered, "\n")...)
		previous = kind
	}
	if len(out) == 0 {
		out = append(out, hintStyle.Render("(nothing said yet)"))
	}
	return out
}

func (m *Model) taskHeading(t *task) string {
	state := "working " + t.elapsed().Round(time.Second).String()
	switch {
	case t.done && t.failed:
		state = "failed after " + t.elapsed().Round(time.Second).String()
	case t.done:
		state = "done in " + t.elapsed().Round(time.Second).String()
	}

	parts := []string{"agent · " + t.what}
	if t.agent != "" {
		parts = append(parts, t.agent)
	}
	parts = append(parts, t.model, state)
	return metaStyle.Render(clamp(strings.Join(parts, " · "), m.contentWidth()))
}

func (m *Model) agentsHeading() string {
	heading := fmt.Sprintf("agents · %d of %d", m.agents.cursor+1, len(m.tasks))
	if running := m.runningTasks(); running > 0 {
		heading += fmt.Sprintf(" · %d working", running)
	}
	return metaStyle.Render(heading)
}

func (m *Model) agentsFooter() string {
	if m.agents.viewing >= 0 {
		return hintStyle.Render("j/k or ↑↓ scroll · g/G ends · x stop it · q back to the list")
	}
	return hintStyle.Render("j/k or ↑↓ move · ⏎ walk in · x stop · g/G ends · q back")
}

func (m *Model) agentsWindow(rows int) ([]*task, int) {
	if len(m.tasks) <= rows {
		return m.tasks, m.agents.cursor
	}
	start := min(max(m.agents.cursor-rows/2, 0), len(m.tasks)-rows)
	return m.tasks[start : start+rows], m.agents.cursor - start
}

func (m *Model) agentsBar(row int) string {
	if row == m.agents.barAt {
		return cursorBar()
	}
	if life, ok := m.agents.trail[row]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return noBar()
}

func (m *Model) aimAgentsBar() tea.Cmd {
	_, cursor := m.agentsWindow(m.agentsRows())
	m.agents.barTo = cursor
	if !m.motion.on {
		m.agents.barAt = cursor
		return nil
	}
	return m.animate()
}

func (m *Model) advanceAgentsBar() bool {
	if !m.agents.open || m.agents.viewing >= 0 || m.agents.barAt == m.agents.barTo {
		return m.fadeAgentsTrail()
	}

	m.leaveAgentsTrail(m.agents.barAt)
	if m.agents.barAt < m.agents.barTo {
		m.agents.barAt++
	} else {
		m.agents.barAt--
	}
	return true
}

func (m *Model) leaveAgentsTrail(row int) {
	if m.agents.trail == nil {
		m.agents.trail = map[int]int{}
	}
	m.agents.trail[row] = trailLife()
}

func (m *Model) fadeAgentsTrail() bool {
	if len(m.agents.trail) == 0 {
		return false
	}
	for row, life := range m.agents.trail {
		if life <= 1 {
			delete(m.agents.trail, row)
			continue
		}
		m.agents.trail[row] = life - 1
	}
	return true
}
