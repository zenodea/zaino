package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/tool"
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
			name:    "permission",
			aliases: []string{"perm", "mode"},
			arg:     "[mode]",
			summary: "show or set when tools stop to ask",
			run:     cmdPermission,
		},
		{
			name:    "tools",
			summary: "list the tools the model has",
			run:     cmdTools,
		},
		{
			name:    "vim",
			arg:     "[on|off]",
			summary: "modal editing in the composer",
			run:     cmdVim,
		},
		{
			name:    "compact",
			summary: "fold the conversation so far into a summary",
			run:     cmdCompact,
		},
		{
			name:    "limit",
			arg:     "[tokens|off]",
			summary: "stop the session when the context passes a ceiling",
			run:     cmdLimit,
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
		width = max(width, lipgloss.Width(labels[i]))
	}

	lines := make([]string, 0, len(all)+8)
	for i, c := range all {
		row := menuPickStyle.Render(pad(labels[i], width+2)) + bodyStyle.Render(c.summary)
		if len(c.aliases) > 0 {
			row += hintStyle.Render("  (/" + strings.Join(c.aliases, ", /") + ")")
		}
		lines = append(lines, row)
	}

	lines = append(lines, "", metaStyle.Render("keys"), "")
	for _, k := range [][2]string{
		{"⏎", "send · open a tool call under the bar"},
		{"⌥⏎", "newline"},
		{"⌃j ⌃k", "walk the transcript"},
		{"↑ ↓", "earlier prompts"},
		{"⇧⇥", "cycle the permission mode"},
		{"esc", "stop the turn, once vim is done with it"},
		{"⌃c", "stop the turn · twice to leave"},
	} {
		lines = append(lines, keyCapStyle.Render(pad(k[0], width+2))+hintStyle.Render(k[1]))
	}
	return m.show("commands", lines)
}

func cmdClear(m *Model, arg string) tea.Cmd {
	if len(m.messages) == 0 {
		m.notice("nothing to forget")
		return nil
	}
	if arg == "!" {
		m.reset()
		return nil
	}

	return m.ask(chooser{title: "clear · start the conversation over", current: "keep",
		options: []choice{
			{label: "keep it", value: "keep", detail: "leave everything as it is", body: []string{
				hintStyle.Render("Nothing happens."),
			}},
			{label: "clear", value: "clear", detail: whatGoes(m), body: []string{
				keyed("messages", fmt.Sprintf("%d leave the context", len(m.messages))),
				keyed("tokens", humanTokens(m.sessionUsage.InputTokens+m.sessionUsage.OutputTokens)+" spent so far"),
				"",
				hintStyle.Render("The transcript stays readable and the session file keeps every line."),
				hintStyle.Render("A clear marks where the context starts, it does not delete anything."),
			}},
		},
		apply: func(m *Model, picked choice) {
			if picked.value == "clear" {
				m.reset()
			}
		}})
}

func whatGoes(m *Model) string {
	return fmt.Sprintf("%d messages stop being sent", len(m.messages))
}

func cmdModel(m *Model, arg string) tea.Cmd {
	if arg == "" {
		// Nothing compiled in — OpenRouter fronts hundreds — so ask the API
		// and open the list once it answers.
		if len(m.knownModels()) == 0 {
			if cmd := fetchModels(m.agent.Provider); cmd != nil {
				m.awaitingModels = true
				m.notice("asking %s for its models…", m.provider)
				return cmd
			}
			m.notice("model: %s\n%s does not list its models — pass one to /model",
				m.modelName(), m.provider)
			return nil
		}
		// Open on what is known now, and refresh from the API behind it.
		return tea.Batch(m.openModelChooser(), fetchModels(m.agent.Provider))
	}

	m.agent.Model = arg
	m.lastModel = "" // the header prefers what the last turn reported
	m.record(session.Model(m.provider, arg))
	m.notice("model → %s", arg)
	return nil
}

func (m *Model) openModelChooser() tea.Cmd {
	options := []choice{{
		label: "provider default", value: "",
		detail: m.agent.Provider.DefaultModel(),
		body:   []string{hintStyle.Render("Whatever " + m.provider + " picks for itself, which follows their default")},
	}}
	for _, id := range m.knownModels() {
		options = append(options, choice{label: id, value: id, body: []string{
			keyed("provider", m.provider),
			keyed("effort", orDefault(m.agent.Effort, "provider default")),
			"",
			hintStyle.Render("Changing model keeps the conversation; only what answers it changes."),
		}})
	}
	return m.ask(chooser{title: "model · " + m.provider, options: options, current: m.agent.Model,
		apply: func(m *Model, picked choice) {
			m.agent.Model = picked.value
			m.lastModel = ""
			m.record(session.Model(m.provider, picked.value))
			m.notice("model → %s", m.modelName())
		}})
}

func cmdProvider(m *Model, arg string) tea.Cmd {
	if arg == "" {
		var options []choice
		for _, name := range provider.Available() {
			detail, body := credentialState(name), []string{
				hintStyle.Render("Switching clears the context. A model id, thinking signatures"),
				hintStyle.Render("and tool ids mean nothing on the other side."),
			}
			if !provider.HasCredentials(name) {
				body = []string{
					hintStyle.Render("Nothing here can reach " + name + " yet."),
					hintStyle.Render("Choosing it opens the ways of setting it up."),
				}
			}
			if name == m.provider {
				detail, body = "in use", []string{
					keyed("model", m.modelName()),
					keyed("messages", fmt.Sprintf("%d in context", len(m.messages))),
					"",
					hintStyle.Render("Already here. Choosing it again changes nothing."),
				}
			}
			options = append(options, choice{label: name, detail: detail, value: name, body: body})
		}
		return m.ask(chooser{title: "provider", options: options, current: m.provider,
			apply: func(m *Model, picked choice) { cmdProvider(m, picked.value) }})
	}

	backend, err := provider.New(arg)
	if err != nil {
		// Not a dead end: offer the ways of authenticating instead of just
		// reporting that there is no key.
		if slices.Contains(provider.Available(), arg) {
			return m.setUpProvider(arg)
		}
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

var effortDetail = map[string]string{
	llm.EffortLow:    "answers fastest, thinks least",
	llm.EffortMedium: "a working balance",
	llm.EffortHigh:   "thinks before answering",
	llm.EffortXHigh:  "takes its time on hard things",
	llm.EffortMax:    "slowest, and the most thorough",
}

var efforts = []string{llm.EffortLow, llm.EffortMedium, llm.EffortHigh, llm.EffortXHigh, llm.EffortMax}

func cmdEffort(m *Model, arg string) tea.Cmd {
	if arg == "" {
		options := []choice{{label: "default", detail: "whatever the provider does on its own", value: ""}}
		for i, level := range efforts {
			options = append(options, choice{
				label: level, value: level, level: i + 1, detail: effortDetail[level],
			})
		}
		if m.provider == "gemini" {
			options[0].detail = m.provider + " ignores effort"
		}
		return m.ask(chooser{title: "effort · how hard the model works before answering",
			layout: layoutScale, options: options, current: m.agent.Effort,
			apply: func(m *Model, picked choice) {
				m.agent.Effort = picked.value
				m.record(session.Effort(picked.value))
				m.notice("effort → %s", orDefault(picked.value, "provider default"))
			}})
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
		return m.ask(chooser{title: "thinking · whether the model shows its working",
			current: onOff(t.Show), options: []choice{
				{label: "shown", value: "shown", detail: "reasoning is printed as it arrives", body: []string{
					thinkingMarker.Render("⋯ ") + thinkingStyle.Render("the loop calls runTools once per turn, so the gate has to"),
					thinkingMarker.Render("  ") + thinkingStyle.Render("sit inside it rather than around it"),
					"",
					bodyStyle.Render("The gate belongs inside the loop."),
					"",
					hintStyle.Render("You see what it considered, which is worth having when it is wrong."),
					hintStyle.Render("It is requested either way, so this costs no tokens to turn on."),
				}},
				{label: "hidden", value: "hidden", detail: "requested, but kept off screen", body: []string{
					bodyStyle.Render("The gate belongs inside the loop."),
					"",
					hintStyle.Render("Only the answer. The reasoning is still asked for and still"),
					hintStyle.Render("billed — this changes what reaches the screen, nothing else."),
				}},
			}, apply: func(m *Model, picked choice) {
				cmdThinking(m, map[string]string{"shown": "on", "hidden": "off"}[picked.value])
			}})
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
	case arg == "-":
		m.agent.System = ""
		m.record(session.System(""))
		m.notice("system prompt dropped")
		return nil

	case arg != "":
		m.agent.System = arg
		m.record(session.System(arg))
		m.notice("system prompt set (%d characters)", len(arg))
		return nil
	}

	if strings.TrimSpace(m.agent.System) == "" {
		return m.show("system prompt", []string{
			hintStyle.Render("none — the model is running on its own instructions"),
			"",
			hintStyle.Render("/system <text>  set one"),
		})
	}

	lines := renderMarkdown(m.agent.System, max(m.contentWidth()-2, 30), bodyStyle)
	return m.show(fmt.Sprintf("system prompt · %d characters", len(m.agent.System)),
		append(strings.Split(lines, "\n"), "", hintStyle.Render("/system <text> replaces it · /system - drops it")))
}

func cmdPermission(m *Model, arg string) tea.Cmd {
	gate := m.agent.Gate
	if gate == nil || gate.Policy == nil {
		m.notice("permission: nothing is gated")
		return nil
	}

	if arg == "" {
		var options []choice
		for _, name := range permission.ModeNames() {
			mode := permission.Mode(name)
			options = append(options, choice{
				label: name, detail: mode.Describe(), value: name, grid: capabilities(mode),
			})
		}
		return m.ask(chooser{title: "permission · what the agent may do without stopping to ask",
			layout: layoutBoard, options: options, current: string(gate.Mode()),
			apply: func(m *Model, picked choice) {
				mode, err := permission.ParseMode(picked.value)
				if err != nil {
					m.push(entry{kind: entryError, text: err.Error()})
					return
				}
				gate.Policy.SetMode(mode)
				m.notice("permission → %s", mode)
			}})
	}

	mode, err := permission.ParseMode(arg)
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}
	gate.Policy.SetMode(mode)
	m.notice("permission → %s · %s", mode, mode.Describe())
	return nil
}

func cmdTools(m *Model, _ string) tea.Cmd {
	if len(m.agent.Tools) == 0 {
		m.notice("no tools — the model can only talk")
		return nil
	}

	width := 0
	for _, t := range m.agent.Tools {
		width = max(width, lipgloss.Width(t.Definition().Name))
	}

	lines := []string{
		hintStyle.Render("● goes through   ? stops to ask   ✕ refused") +
			metaStyle.Render("   under "+string(m.agent.Gate.Mode())),
		"",
	}
	for _, t := range m.agent.Tools {
		def := t.Definition()
		state := stateFor(m, tool.ActionOf(t))

		lines = append(lines,
			stateMark(state)+" "+menuPickStyle.Render(pad(def.Name, width+2))+
				bodyStyle.Render(firstSentence(def.Description)))
	}
	return m.show(fmt.Sprintf("tools · %d", len(m.agent.Tools)), lines)
}

func stateFor(m *Model, action permission.Action) int {
	gate := m.agent.Gate
	if gate == nil || gate.Policy == nil {
		return stateAllow
	}
	switch verdict, _ := gate.Policy.Decide(permission.Request{Action: action}); verdict {
	case permission.Allow:
		return stateAllow
	case permission.Deny:
		return stateRefuse
	}
	return stateAsk
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func cmdVim(m *Model, arg string) tea.Cmd {
	switch arg {
	case "":
		return m.ask(chooser{title: "vim · how the composer handles keys",
			current: yesNo(m.vimEnabled()), options: []choice{
				{label: "on", value: "on", detail: "starts in insert, esc for normal mode", body: []string{
					keyed("motions", "h j k l w b e 0 ^ $ gg G, with counts: 3w 2j"),
					keyed("insert", "i a I A o O"),
					keyed("edits", "x X D C dd cc dw cw db d$ d0 de"),
					keyed("visual", "v select · V by line · d c y on it · esc cancel"),
					keyed("other", "u undo · p P paste what you last cut"),
					"",
					hintStyle.Render("⏎ always sends, in either mode. Nothing changes until you press esc."),
				}},
				{label: "off", value: "off", detail: "a plain text box", body: []string{
					keyed("move", "← → ↑ ↓ · ⌥← ⌥→ by word · ⌃a ⌃e ends of the line"),
					keyed("edit", "⌃w delete a word back · ⌃k to the end · ⌃u to the start"),
					"",
					hintStyle.Render("esc stops a running turn straight away instead of changing mode."),
				}},
			}, apply: func(m *Model, picked choice) { cmdVim(m, picked.value) }})
	case "on", "true":
		m.UseVim(true)
		m.notice("vim → on (esc for normal mode)")
	case "off", "false":
		m.UseVim(false)
		m.notice("vim → off")
	default:
		m.push(entry{kind: entryError, text: fmt.Sprintf("usage: /vim [on|off], got %q", arg)})
	}
	return nil
}

func cmdCompact(m *Model, arg string) tea.Cmd {
	if m.streaming {
		m.notice("wait for the turn to finish")
		return nil
	}
	if len(m.messages) < 2 {
		m.notice("nothing to compact yet")
		return nil
	}
	if arg != "!" {
		return m.ask(chooser{title: "compact · fold what came before into a summary", current: "keep",
			options: []choice{
				{label: "keep it", value: "keep", detail: "leave the conversation whole", body: []string{
					hintStyle.Render("Nothing happens. It will fold on its own when the window fills."),
				}},
				{label: "compact", value: "compact",
					detail: fmt.Sprintf("%d messages become one summary", len(m.messages)),
					body: []string{
						keyed("now", humanTokens(m.agent.Used())+" of "+humanTokens(max(m.agent.Window(), 1))),
						keyed("kept", "the most recent stretch, whole"),
						keyed("cost", "one turn, to write the summary"),
						"",
						hintStyle.Render("The session file keeps everything; only what is sent gets shorter."),
					}},
			},
			apply: func(m *Model, picked choice) {
				if picked.value == "compact" {
					cmdCompact(m, "!")
				}
			}})
	}

	m.streaming = true
	m.startedAt = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.agent.Hooks = m.hooks(ctx)

	messages := m.messages
	go func() {
		kept, err := m.agent.Fold(ctx, messages)
		m.events <- doneMsg{Messages: kept, Err: err}
	}()
	return m.spinner.Tick
}

func cmdLimit(m *Model, arg string) tea.Cmd {
	if arg == "" {
		return m.ask(chooser{title: "limit · how much context this session may reach", current: limitValue(m.agent.MaxContext),
			options: []choice{
				{label: "off", value: "off", detail: "no ceiling", body: []string{
					hintStyle.Render("The window still folds on its own once it fills."),
				}},
				{label: "100k", value: "100000", detail: "stop at 100,000 tokens", body: limitBody(m, 100_000)},
				{label: "200k", value: "200000", detail: "stop at 200,000 tokens", body: limitBody(m, 200_000)},
				{label: "500k", value: "500000", detail: "stop at 500,000 tokens", body: limitBody(m, 500_000)},
			},
			apply: func(m *Model, picked choice) { cmdLimit(m, picked.value) }})
	}

	limit, err := agent.ParseTokens(arg)
	if err != nil {
		m.push(entry{kind: entryError, text: fmt.Sprintf("usage: /limit [tokens|off], got %q", arg)})
		return nil
	}

	m.agent.MaxContext = limit
	m.record(session.Limit(limit))
	if limit == 0 {
		m.notice("limit → off")
		return nil
	}
	m.notice("limit → %s · the turn that would pass it stops instead", humanTokens(limit))
	return nil
}

func limitValue(limit int) string {
	if limit == 0 {
		return "off"
	}
	return strconv.Itoa(limit)
}

func limitBody(m *Model, limit int) []string {
	body := []string{keyed("now", humanTokens(m.agent.Used())+" in context")}
	if m.agent.Used() > limit {
		body = append(body, keyed("effect", "the next turn stops straight away"))
	}
	return append(body, "",
		hintStyle.Render("The turn is held before it is sent, and ⏎ sends it anyway."))
}

func cmdUsage(m *Model, _ string) tea.Cmd {
	u := m.sessionUsage
	width := min(max(m.contentWidth()-34, 12), 40)

	lines := []string{}
	if ceiling := m.agent.Ceiling(); ceiling > 0 {
		used := m.agent.Used()
		lines = append(lines,
			bar("context", used, ceiling, width, contextHeat(used, ceiling))+
				metaStyle.Render(" of "+humanTokens(ceiling)),
			hintStyle.Render(pad("", 13)+m.ceilingNote(used, ceiling)),
			"")
	}

	biggest := max(u.InputTokens, max(u.OutputTokens, max(u.ThinkingTokens, u.CacheReadTokens)))
	lines = append(lines,
		bar("input", u.InputTokens, biggest, width, meterHeat[2]),
		bar("output", u.OutputTokens, biggest, width, meterHeat[1]),
	)
	if u.ThinkingTokens > 0 {
		lines = append(lines, bar("thinking", u.ThinkingTokens, biggest, width, meterHeat[0]))
	}
	if u.CacheReadTokens > 0 {
		lines = append(lines, bar("cache read", u.CacheReadTokens, biggest, width, meterHeat[3]))
	}
	if u.CacheWriteTokens > 0 {
		lines = append(lines, bar("cache write", u.CacheWriteTokens, biggest, width, meterHeat[3]))
	}

	lines = append(lines, "", hintStyle.Render(fmt.Sprintf("%d messages · %s · %s",
		len(m.messages), m.modelName(), orDefault(m.sessionID(), "not saved"))))
	return m.show("usage · "+m.provider, lines)
}

func contextHeat(used, window int) lipgloss.Style {
	switch share := used * 100 / max(window, 1); {
	case share > 85:
		return meterHeat[4]
	case share > 60:
		return meterHeat[3]
	}
	return meterHeat[1]
}

// What happens when the bar fills depends on which bound is nearer.
func (m *Model) ceilingNote(used, ceiling int) string {
	if ceiling == m.agent.MaxContext && m.agent.MaxContext > 0 {
		if used*100/max(ceiling, 1) > 85 {
			return "the turn that passes " + humanTokens(ceiling) + " stops before it is sent"
		}
		return fmt.Sprintf("%d%% of the %s limit", used*100/max(ceiling, 1), humanTokens(ceiling))
	}
	return contextNote(used, ceiling)
}

func contextNote(used, window int) string {
	switch share := used * 100 / max(window, 1); {
	case used == 0:
		return "nothing sent yet"
	case share > 85:
		return "the next turn will fold the older half into a summary"
	default:
		return fmt.Sprintf("%d%% of the window", share)
	}
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

func yesNo(v bool) string {
	if v {
		return "on"
	}
	return "off"
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

// Asked of a real policy rather than written down, so the grid cannot claim
// something the gate would not do.
func capabilities(mode permission.Mode) []gridCell {
	policy := permission.NewPolicy(mode)

	actions := []struct {
		label  string
		action permission.Action
	}{
		{"read", permission.Read},
		{"write", permission.Write},
		{"run", permission.Execute},
		{"fetch", permission.Network},
	}

	cells := make([]gridCell, 0, len(actions))
	for _, a := range actions {
		state := stateAsk
		switch verdict, _ := policy.Decide(permission.Request{Action: a.action}); verdict {
		case permission.Allow:
			state = stateAllow
		case permission.Deny:
			state = stateRefuse
		}
		cells = append(cells, gridCell{label: a.label, state: state})
	}
	return cells
}

func keyed(label, value string) string {
	return metaStyle.Render(pad(label, 10)) + bodyStyle.Render(value)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
