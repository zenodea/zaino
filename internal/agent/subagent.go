package agent

import (
	"fmt"
	"strings"

	"github.com/zenodea/zaino/internal/tool"
)

type Subagent struct {
	Name        string
	Description string
	Model       string
	Tools       []string
	System      string
}

func (a *Agent) subagent(name string) (Subagent, error) {
	for _, s := range a.Subagents {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	if len(a.Subagents) == 0 {
		return Subagent{}, fmt.Errorf("no agent named %q — there are none defined, so leave it out", name)
	}
	return Subagent{}, fmt.Errorf("no agent named %q — have %s", name, strings.Join(a.subagentNames(), ", "))
}

func (a *Agent) subagentNames() []string {
	out := make([]string, len(a.Subagents))
	for i, s := range a.Subagents {
		out[i] = s.Name
	}
	return out
}

func (s Subagent) toolbox(inherited []tool.Tool) ([]tool.Tool, error) {
	if len(s.Tools) == 0 {
		return inherited, nil
	}
	return tool.Select(inherited, s.Tools, nil)
}
