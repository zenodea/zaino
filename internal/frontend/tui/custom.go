package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/attach"
	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func (m *Model) UseConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	m.cfg = cfg

	built := commandList()
	for _, c := range cfg.Commands {
		// A file cannot take a name zaino already answers to: /quit has to
		// keep meaning quit.
		if _, taken := findCommand(built, c.Name); taken {
			m.notice("%s is named after a command zaino already has, so it is not loaded", c.Path)
			continue
		}
		m.custom = append(m.custom, fromFile(c))
	}
}

func (m *Model) commands() []command {
	return append(commandList(), m.custom...)
}

// What can be run right now. A turn in flight narrows it to the commands that
// only look: /usage and /agents are most wanted while the model is busy.
func (m *Model) available() []command {
	all := m.commands()
	if !m.streaming {
		return all
	}
	live := all[:0:0]
	for _, c := range all {
		if c.live {
			live = append(live, c)
		}
	}
	return live
}

func (m *Model) known(name string) bool {
	_, ok := findCommand(m.commands(), name)
	return ok
}

// Whether what is in the composer may run during a turn.
func (m *Model) typedLive() bool {
	line := strings.TrimSpace(m.input.Value())
	if !isCommandLine(line) {
		return false
	}
	name, _ := splitCommand(line)
	c, ok := findCommand(m.commands(), name)
	return ok && c.live
}

func fromFile(c config.Command) command {
	summary := c.Description
	if summary == "" {
		summary = "your prompt, from " + filepath.Base(c.Path)
	}
	return command{
		name:    c.Name,
		arg:     "[text]",
		summary: summary,
		run: func(m *Model, arg string) tea.Cmd {
			if c.Nothing != "" && !m.saidSomething() {
				m.notice("%s", c.Nothing)
				return nil
			}
			return m.send("/"+c.Name+" "+arg, c.Expand(arg))
		},
	}
}

// Sent the way a typed prompt is, except that the transcript can say what was
// asked for rather than the whole of what went out.
func (m *Model) send(display, prompt string) tea.Cmd {
	if m.streaming {
		m.notice("still answering — wait for this turn to finish")
		return nil
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		m.notice("that command has nothing in it")
		return nil
	}

	content, attached, err := attach.Prompt(m.workdir(), prompt)
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}

	shown := strings.TrimSpace(display)
	for _, what := range attached {
		shown += "\n" + attachedStyle.Render("⧉ "+what)
	}

	m.push(entry{kind: entryUser, text: shown})
	m.messages = append(m.messages, llm.Message{Role: llm.RoleUser, Content: content})
	return m.launch()
}

func (m *Model) workdir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func (m *Model) saidSomething() bool {
	for _, msg := range m.messages {
		if msg.Role == llm.RoleAssistant && strings.TrimSpace(msg.Text()) != "" {
			return true
		}
	}
	return false
}

func cmdProfile(m *Model, arg string) tea.Cmd {
	profiles := m.profiles()
	if len(profiles) == 0 {
		m.notice("no profiles — name some under \"profiles\" in your config")
		return nil
	}

	if arg == "" {
		options := make([]choice, 0, len(profiles))
		for _, name := range profiles {
			p := m.cfg.Profiles[name]
			options = append(options, choice{
				label: name, value: name, detail: profileDetail(p),
				body: []string{
					keyed("provider", orDefault(p.Provider, "unchanged")),
					keyed("model", orDefault(p.Model, "unchanged")),
					keyed("effort", orDefault(p.Effort, "unchanged")),
					"",
					hintStyle.Render("Switching provider clears the context; the rest keeps it."),
				},
			})
		}
		return m.ask(chooser{title: "profile", options: options,
			apply: func(m *Model, picked choice) { m.runCmd(cmdProfile(m, picked.value)) }})
	}

	p, ok := m.cfg.Profiles[arg]
	if !ok {
		m.push(entry{kind: entryError, text: fmt.Sprintf(
			"no profile named %q — have %s", arg, strings.Join(profiles, ", "))})
		return nil
	}

	var cmd tea.Cmd
	if p.Provider != "" && p.Provider != m.provider {
		cmd = cmdProvider(m, p.Provider)
	}
	if p.Model != "" {
		m.agent.Model = p.Model
		m.lastModel = ""
		m.record(session.Model(m.provider, p.Model))
	}
	if p.MaxTokens != 0 {
		m.agent.MaxTokens = p.MaxTokens
	}
	if p.Effort != "" {
		m.agent.Effort = p.Effort
		m.record(session.Effort(p.Effort))
	}
	if p.System != "" {
		m.agent.System = p.System
		m.record(session.System(p.System))
	}
	if p.Thinking != nil && m.agent.Thinking != nil {
		m.agent.Thinking.Show = *p.Thinking
		m.record(session.Thinking(*p.Thinking))
	}
	m.notice("profile → %s · %s", arg, profileDetail(p))
	return cmd
}

func profileDetail(p config.Profile) string {
	var parts []string
	for _, part := range []string{p.Provider, p.Model, p.Effort} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "changes nothing on its own"
	}
	return strings.Join(parts, " · ")
}

func (m *Model) profiles() []string {
	if m.cfg == nil {
		return nil
	}
	names := make([]string, 0, len(m.cfg.Profiles))
	for name := range m.cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cmdConfig(m *Model, _ string) tea.Cmd {
	if m.cfg == nil {
		return m.show("config", []string{hintStyle.Render("running on flags alone")})
	}

	lines := []string{
		keyed("provider", m.provider),
		keyed("model", m.modelName()),
		keyed("effort", orDefault(m.agent.Effort, "provider default")),
		keyed("permission", string(m.agent.Gate.Mode())),
		keyed("system", describeLength(m.agent.System)),
		keyed("project", describeLength(m.agent.Project)),
	}

	if allow, deny := m.rules(); len(allow)+len(deny) > 0 {
		lines = append(lines, "")
		if len(allow) > 0 {
			lines = append(lines, keyed("allow", strings.Join(allow, ", ")))
		}
		if len(deny) > 0 {
			lines = append(lines, keyed("deny", strings.Join(deny, ", ")))
		}
	}

	if names := m.profiles(); len(names) > 0 {
		lines = append(lines, "", keyed("profiles", strings.Join(names, ", ")))
	}
	if len(m.custom) > 0 {
		names := make([]string, len(m.custom))
		for i, c := range m.custom {
			names[i] = "/" + c.name
		}
		lines = append(lines, keyed("commands", strings.Join(names, ", ")))
	}
	if len(m.cfg.Subagents) > 0 {
		names := make([]string, len(m.cfg.Subagents))
		for i, a := range m.cfg.Subagents {
			names[i] = a.Name
		}
		lines = append(lines, keyed("agents", strings.Join(names, ", ")))
	}

	lines = append(lines, "", metaStyle.Render("read from"), "")
	from := append(append([]string(nil), m.cfg.Sources...), m.cfg.MCP...)
	if len(from) == 0 {
		lines = append(lines, hintStyle.Render("nothing — no config files exist yet"))
	}
	for _, path := range from {
		lines = append(lines, hintStyle.Render(path))
	}
	lines = append(lines,
		"",
		hintStyle.Render("Later files win, and a flag wins over every file."))
	return m.show("config", lines)
}

func (m *Model) rules() (allow, deny []string) {
	if m.agent.Gate == nil || m.agent.Gate.Policy == nil {
		return nil, nil
	}
	a, d := m.agent.Gate.Policy.Rules()
	for _, rule := range a {
		allow = append(allow, rule.String())
	}
	for _, rule := range d {
		deny = append(deny, rule.String())
	}
	return allow, deny
}

func describeLength(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return fmt.Sprintf("%d characters", len(s))
}
