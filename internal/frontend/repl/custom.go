package repl

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func findPrompt(cfg *config.Config, name string) (config.Command, bool) {
	if cfg == nil {
		return config.Command{}, false
	}
	for _, c := range cfg.Commands {
		if c.Name == name {
			return c, true
		}
	}
	return config.Command{}, false
}

func helpWith(cfg *config.Config) string {
	if cfg == nil {
		return help
	}

	lines := []string{help}
	for _, c := range cfg.Commands {
		if strings.Contains(help, "/"+c.Name+" ") {
			continue
		}
		summary := c.Description
		if summary == "" {
			summary = "your prompt, from " + filepath.Base(c.Path)
		}
		lines = append(lines, fmt.Sprintf("%-20s %s", "/"+c.Name, summary))
	}
	return strings.Join(lines, "\n")
}

func answered(messages []llm.Message) bool {
	for _, msg := range messages {
		if msg.Role == llm.RoleAssistant && strings.TrimSpace(msg.Text()) != "" {
			return true
		}
	}
	return false
}

func useProfile(ag *agent.Agent, name string, o Options) error {
	if o.Config == nil || len(o.Config.Profiles) == 0 {
		return fmt.Errorf("no profiles — name some under \"profiles\" in your config")
	}
	if name == "" {
		return fmt.Errorf("profiles: %s", strings.Join(profileNames(o.Config), ", "))
	}

	p, ok := o.Config.Profiles[name]
	if !ok {
		return fmt.Errorf("no profile named %q — have %s", name, strings.Join(profileNames(o.Config), ", "))
	}
	if p.Provider != "" && p.Provider != ag.Provider.Name() {
		return fmt.Errorf("%s wants %s — switch with /provider %s first, which clears the context",
			name, p.Provider, p.Provider)
	}

	if p.Model != "" {
		ag.Model = p.Model
		o.Recorder.Append(session.Model(ag.Provider.Name(), p.Model))
	}
	if p.MaxTokens != 0 {
		ag.MaxTokens = p.MaxTokens
	}
	if p.Effort != "" {
		ag.Effort = p.Effort
		o.Recorder.Append(session.Effort(p.Effort))
	}
	if p.System != "" {
		ag.System = p.System
		o.Recorder.Append(session.System(p.System))
	}
	if p.Thinking != nil && ag.Thinking != nil {
		ag.Thinking.Show = *p.Thinking
		o.Recorder.Append(session.Thinking(*p.Thinking))
	}
	return nil
}

func profileNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func describeConfig(ag *agent.Agent, o Options) string {
	if o.Config == nil {
		return "running on flags alone"
	}

	lines := []string{
		"provider: " + ag.Provider.Name(),
		"model: " + modelName(ag),
		"system: " + describeLength(ag.System),
		"project: " + describeLength(ag.Project),
	}
	if o.Gate != nil && o.Gate.Policy != nil {
		allow, deny := o.Gate.Policy.Rules()
		if len(allow) > 0 {
			lines = append(lines, "allow: "+ruleList(allow))
		}
		if len(deny) > 0 {
			lines = append(lines, "deny: "+ruleList(deny))
		}
	}
	if names := profileNames(o.Config); len(names) > 0 {
		lines = append(lines, "profiles: "+strings.Join(names, ", "))
	}
	if names := commandNames(o.Config); names != "" {
		lines = append(lines, "commands: "+names)
	}
	if len(o.Config.Subagents) > 0 {
		names := make([]string, len(o.Config.Subagents))
		for i, a := range o.Config.Subagents {
			names[i] = a.Name
		}
		lines = append(lines, "agents: "+strings.Join(names, ", "))
	}

	from := append(append([]string(nil), o.Config.Sources...), o.Config.MCP...)
	if len(from) == 0 {
		from = []string{"nothing — no config files exist yet"}
	}
	return strings.Join(append(lines, "read from:\n  "+strings.Join(from, "\n  ")), "\n")
}

func commandNames(cfg *config.Config) string {
	names := make([]string, len(cfg.Commands))
	for i, c := range cfg.Commands {
		names[i] = "/" + c.Name
	}
	return strings.Join(names, ", ")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}

func describeLength(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return fmt.Sprintf("%d characters", len(s))
}

func ruleList[T fmt.Stringer](rules []T) string {
	out := make([]string, len(rules))
	for i, rule := range rules {
		out[i] = rule.String()
	}
	return strings.Join(out, ", ")
}

// Printing the prompt back is as close to handing it over as a line-based
// REPL gets.
func rewind(ag *agent.Agent, messages []llm.Message, arg string, o Options,
	notice, fail func(string, ...any)) ([]llm.Message, bool) {
	turns := agent.Turns(messages)
	if len(turns) == 0 {
		notice("nothing to go back to — you have not asked anything yet")
		return nil, false
	}

	if arg == "" {
		lines := make([]string, 0, len(turns)+1)
		lines = append(lines, "rewind · pick one with /rewind <n>")
		for i, t := range turns {
			lines = append(lines, fmt.Sprintf("%3d  %s", i+1, truncate(firstLine(t.Prompt), 56)))
		}
		notice("%s", strings.Join(lines, "\n"))
		return nil, false
	}

	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > len(turns) {
		fail("/rewind takes a number from 1 to %d", len(turns))
		return nil, false
	}

	t := turns[n-1]
	if err := o.Recorder.Rewind(t.At); err != nil {
		fail("%s", err)
		return nil, false
	}
	notice("rewound · %d messages left the context\n› %s", len(messages)-t.At, t.Prompt)
	return messages[:t.At], true
}
