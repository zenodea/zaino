package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/permission"
)

// The flag set by another name, so the config files can reach the same values
// the command line does. A flag that was actually typed is left alone.
type knobs struct {
	provider      *string
	model         *string
	maxTokens     *int
	effort        *string
	system        *string
	thinking      *bool
	permission    *string
	allowOutside  *bool
	tools         *string
	excludeTools  *string
	contextWindow *int
	maxContext    *string
	vim           *bool
	mouse         *bool
	animate       *bool
}

func (k knobs) apply(c *config.Config, profile string, given map[string]bool) error {
	str := func(name string, dst *string, v string) {
		if !given[name] && v != "" {
			*dst = v
		}
	}
	num := func(name string, dst *int, v int) {
		if !given[name] && v != 0 {
			*dst = v
		}
	}
	yes := func(name string, dst *bool, v *bool) {
		if !given[name] && v != nil {
			*dst = *v
		}
	}

	str("provider", k.provider, c.Provider)
	str("model", k.model, c.Model)
	num("max-tokens", k.maxTokens, c.MaxTokens)
	str("effort", k.effort, c.Effort)
	str("system", k.system, c.System)
	str("permission", k.permission, c.Permission)
	str("tools", k.tools, strings.Join(c.Tools, ","))
	str("exclude-tools", k.excludeTools, strings.Join(c.ExcludeTools, ","))
	num("context-window", k.contextWindow, c.ContextWindow)
	str("max-context", k.maxContext, c.MaxContext)
	yes("thinking", k.thinking, c.Thinking)
	yes("allow-outside", k.allowOutside, c.AllowOutside)
	yes("vim", k.vim, c.Vim)
	yes("mouse", k.mouse, c.Mouse)
	yes("animate", k.animate, c.Animate)

	// A profile is a deliberate pick, so it beats the plain keys around it —
	// but still not a flag.
	asked := given["profile"]
	if profile == "" {
		profile, asked = c.Profile, false
	}
	if profile == "" {
		return nil
	}
	p, ok := c.Profiles[profile]
	if !ok {
		return fmt.Errorf("no profile named %q — %s", profile, haveProfiles(c))
	}

	// -profile on the command line is as deliberate as the flags it stands
	// for, so what it sets counts as typed: a resumed session restores what
	// the profile left alone, and not what it changed.
	take := func(name string, has bool, set func()) {
		if !has || given[name] {
			return
		}
		set()
		if asked {
			given[name] = true
		}
	}

	take("provider", p.Provider != "", func() { *k.provider = p.Provider })
	take("model", p.Model != "", func() { *k.model = p.Model })
	take("max-tokens", p.MaxTokens != 0, func() { *k.maxTokens = p.MaxTokens })
	take("effort", p.Effort != "", func() { *k.effort = p.Effort })
	take("system", p.System != "", func() { *k.system = p.System })
	take("thinking", p.Thinking != nil, func() { *k.thinking = *p.Thinking })
	return nil
}

func haveProfiles(c *config.Config) string {
	if len(c.Profiles) == 0 {
		return "your config defines none"
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return "have " + strings.Join(names, ", ")
}

func applyRules(p *permission.Policy, c *config.Config) error {
	allow, err := permission.ParseRules(c.Allow)
	if err != nil {
		return fmt.Errorf("allow: %w", err)
	}
	deny, err := permission.ParseRules(c.Deny)
	if err != nil {
		return fmt.Errorf("deny: %w", err)
	}
	p.SetRules(allow, deny)
	return nil
}

// -system takes the prompt itself, or @ and a file holding one.
func readPrompt(value string) (string, error) {
	path, ok := strings.CutPrefix(value, "@")
	if !ok {
		return value, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("-system: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
