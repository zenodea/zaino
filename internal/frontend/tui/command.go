package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/session"
)

type command struct {
	name    string
	aliases []string
	arg     string
	summary string
	run     func(m *Model, arg string) tea.Cmd
}

// A function, not a variable: /help reads the registry back, and a variable
// initialiser may not.
func commandList() []command {
	return []command{
		{
			name:    "help",
			summary: "list the commands",
			run:     cmdHelp,
		},
		{
			name:    "clear",
			aliases: []string{"new", "reset"},
			summary: "forget the conversation and start over",
			run:     cmdClear,
		},
		{
			name:    "model",
			arg:     "[id]",
			summary: "show or change the model",
			run:     cmdModel,
		},
		{
			name:    "provider",
			arg:     "[name]",
			summary: "show or switch provider (clears the context)",
			run:     cmdProvider,
		},
		{
			name:    "effort",
			arg:     "[level]",
			summary: "show or set output effort",
			run:     cmdEffort,
		},
		{
			name:    "thinking",
			arg:     "[on|off]",
			summary: "show or hide the model's reasoning",
			run:     cmdThinking,
		},
		{
			name:    "system",
			arg:     "[prompt|-]",
			summary: "show, set, or (with -) drop the system prompt",
			run:     cmdSystem,
		},
		{
			name:    "usage",
			summary: "token usage for this session",
			run:     cmdUsage,
		},
		{
			name:    "sessions",
			aliases: []string{"resume"},
			summary: "pick up an earlier conversation",
			run:     cmdSessions,
		},
		{
			name:    "quit",
			aliases: []string{"exit", "q"},
			summary: "leave zaino",
			run:     cmdQuit,
		},
	}
}

// Prompts that merely open with a slash — "/etc/hosts is wrong" — are not
// commands.
func isCommandLine(line string) bool {
	if !strings.HasPrefix(line, "/") {
		return false
	}
	word := commandWord(line)
	if word == "" {
		return false
	}
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func commandWord(line string) string {
	word := strings.TrimPrefix(line, "/")
	if i := strings.IndexAny(word, " \t\n"); i >= 0 {
		word = word[:i]
	}
	return word
}

func splitCommand(line string) (name, arg string) {
	name = commandWord(line)
	rest := strings.TrimPrefix(line, "/"+name)
	return strings.ToLower(name), strings.TrimSpace(rest)
}

func lookupCommand(name string) (command, bool) {
	for _, c := range commandList() {
		if c.name == name {
			return c, true
		}
		for _, alias := range c.aliases {
			if alias == name {
				return c, true
			}
		}
	}
	return command{}, false
}

func matchCommands(pattern string) []command {
	type scored struct {
		command
		score int
	}

	var out []scored
	for _, c := range commandList() {
		best, ok := fuzzyScore(pattern, c.name)
		for _, alias := range c.aliases {
			if score, hit := fuzzyScore(pattern, alias); hit && (!ok || score-1 > best) {
				best, ok = score-1, true
			}
		}
		if ok {
			out = append(out, scored{c, best})
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })

	commands := make([]command, len(out))
	for i, s := range out {
		commands[i] = s.command
	}
	return commands
}

func fuzzyScore(pattern, target string) (int, bool) {
	pattern, target = strings.ToLower(pattern), strings.ToLower(target)
	if pattern == "" {
		return 0, true
	}

	score, run, at := 0, 0, 0
	for i, r := range target {
		if at == len(pattern) || rune(pattern[at]) != r {
			run = 0
			continue
		}
		at++
		run++
		score += 1 + run*4
		if i == 0 {
			score += 10
		}
	}
	if at < len(pattern) {
		return 0, false
	}
	return score - len(target), true
}

func (m *Model) runCommand(line string) tea.Cmd {
	name, arg := splitCommand(line)
	c, ok := lookupCommand(name)
	if !ok {
		m.push(entry{kind: entryError, text: fmt.Sprintf("unknown command /%s — try /help", name)})
		return nil
	}
	return c.run(m, arg)
}

func (m *Model) notice(format string, args ...any) {
	m.push(entry{kind: entryNotice, text: fmt.Sprintf(format, args...)})
}

func cmdHelp(m *Model, _ string) tea.Cmd {
	all := commandList()

	width := 0
	labels := make([]string, len(all))
	for i, c := range all {
		labels[i] = "/" + c.name
		if c.arg != "" {
			labels[i] += " " + c.arg
		}
		width = max(width, len(labels[i]))
	}

	lines := make([]string, 0, len(all)+1)
	for i, c := range all {
		line := labels[i] + strings.Repeat(" ", width-len(labels[i])+2) + c.summary
		if len(c.aliases) > 0 {
			line += "  (/" + strings.Join(c.aliases, ", /") + ")"
		}
		lines = append(lines, line)
	}
	m.notice("%s", strings.Join(lines, "\n"))
	return nil
}

func cmdClear(m *Model, _ string) tea.Cmd {
	m.reset()
	return nil
}

func cmdModel(m *Model, arg string) tea.Cmd {
	if arg == "" {
		m.notice("model: %s", m.modelName())
		if known := m.knownModels(); len(known) > 0 {
			m.notice("known: %s", strings.Join(known, ", "))
		}
		return nil
	}
	m.agent.Model = arg
	m.lastModel = "" // the header prefers what the last turn reported
	m.record(session.Model(m.provider, arg))
	m.notice("model → %s", arg)
	return nil
}

func cmdProvider(m *Model, arg string) tea.Cmd {
	if arg == "" {
		m.notice("provider: %s (available: %s)",
			m.provider, strings.Join(provider.Available(), ", "))
		return nil
	}

	backend, err := provider.New(arg)
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}
	if backend.Name() == m.provider {
		m.notice("provider: already %s", m.provider)
		return nil
	}

	m.agent.Provider = backend
	m.provider = backend.Name()
	// A model id, thinking signatures and tool ids mean nothing over there.
	m.agent.Model = ""
	m.lastModel = ""
	m.record(session.Model(backend.Name(), ""))
	m.clearContext()
	m.notice("provider → %s · %s (context cleared)", backend.Name(), backend.DefaultModel())
	return nil
}

var efforts = []string{llm.EffortLow, llm.EffortMedium, llm.EffortHigh, llm.EffortXHigh, llm.EffortMax}

func cmdEffort(m *Model, arg string) tea.Cmd {
	if arg == "" {
		current := m.agent.Effort
		if current == "" {
			current = "(provider default)"
		}
		m.notice("effort: %s (%s)", current, strings.Join(efforts, ", "))
		return nil
	}

	arg = strings.ToLower(arg)
	if arg == "-" || arg == "default" {
		m.agent.Effort = ""
		m.record(session.Effort(""))
		m.notice("effort → provider default")
		return nil
	}
	for _, level := range efforts {
		if arg == level {
			m.agent.Effort = arg
			m.record(session.Effort(arg))
			m.notice("effort → %s", arg)
			if m.provider != "anthropic" {
				m.notice("note: %s ignores effort", m.provider)
			}
			return nil
		}
	}
	m.push(entry{kind: entryError, text: fmt.Sprintf(
		"unknown effort %q — pick one of %s", arg, strings.Join(efforts, ", "))})
	return nil
}

func cmdThinking(m *Model, arg string) tea.Cmd {
	if m.agent.Thinking == nil {
		m.agent.Thinking = &llm.Thinking{Enabled: true}
	}
	t := m.agent.Thinking

	switch strings.ToLower(arg) {
	case "":
		m.notice("thinking: %s", onOff(t.Show))
	case "on", "show", "true":
		t.Show = true
		m.record(session.Thinking(true))
		m.notice("thinking → shown")
	case "off", "hide", "false":
		t.Show = false
		m.record(session.Thinking(false))
		m.notice("thinking → hidden")
	default:
		m.push(entry{kind: entryError, text: fmt.Sprintf("usage: /thinking [on|off], got %q", arg)})
	}
	return nil
}

func cmdSystem(m *Model, arg string) tea.Cmd {
	switch {
	case arg == "":
		if m.agent.System == "" {
			m.notice("system: (none)")
			return nil
		}
		m.notice("system: %s", m.agent.System)
	case arg == "-":
		m.agent.System = ""
		m.record(session.System(""))
		m.notice("system prompt dropped")
	default:
		m.agent.System = arg
		m.record(session.System(arg))
		m.notice("system prompt set (%d chars)", len(arg))
	}
	return nil
}

func cmdUsage(m *Model, _ string) tea.Cmd {
	u := m.sessionUsage

	rows := [][2]string{
		{"input", strconv.Itoa(u.InputTokens)},
		{"output", strconv.Itoa(u.OutputTokens)},
	}
	if u.ThinkingTokens > 0 {
		rows = append(rows, [2]string{"thinking", strconv.Itoa(u.ThinkingTokens)})
	}
	if u.CacheReadTokens > 0 {
		rows = append(rows, [2]string{"cache read", strconv.Itoa(u.CacheReadTokens)})
	}
	if u.CacheWriteTokens > 0 {
		rows = append(rows, [2]string{"cache write", strconv.Itoa(u.CacheWriteTokens)})
	}
	rows = append(rows, [2]string{"messages", strconv.Itoa(len(m.messages))})
	if id := m.sessionID(); id != "" {
		rows = append(rows, [2]string{"session", id})
	}

	label, value := 0, 0
	for _, r := range rows {
		label = max(label, len(r[0]))
		value = max(value, len(r[1]))
	}

	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, "usage · "+m.provider+" · "+m.modelName())
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("  %-*s  %*s", label, r[0], value, r[1]))
	}
	m.notice("%s", strings.Join(lines, "\n"))
	return nil
}

func cmdSessions(m *Model, arg string) tea.Cmd {
	if arg != "" {
		m.resume(arg)
		return nil
	}
	m.openPicker()
	return nil
}

func cmdQuit(m *Model, _ string) tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func onOff(v bool) string {
	if v {
		return "shown"
	}
	return "hidden"
}

func (m *Model) modelName() string {
	switch {
	case m.lastModel != "":
		return m.lastModel
	case m.agent.Model != "":
		return m.agent.Model
	case m.agent.Provider != nil:
		return m.agent.Provider.DefaultModel()
	default:
		return "unknown"
	}
}

func (m *Model) knownModels() []string {
	lister, ok := m.agent.Provider.(llm.ModelLister)
	if !ok {
		return nil
	}
	return lister.Models()
}
