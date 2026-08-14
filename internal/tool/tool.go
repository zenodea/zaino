package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Tool interface {
	Definition() llm.Tool
	Prepare(input json.RawMessage) (Call, error)
}

type Call interface {
	Request() permission.Request
	Run(ctx context.Context) (string, error)
}

// Asks is optional: a tool that always asks under the same action can say so
// without a call being prepared, which is what lets /tools show it.
type Asks interface {
	Action() permission.Action
}

func ActionOf(t Tool) permission.Action {
	if a, ok := t.(Asks); ok {
		return a.Action()
	}
	return permission.Execute
}

func All(w *Workspace) []Tool {
	return []Tool{
		&Read{w}, &Write{w}, &Edit{w}, &Ls{w}, &Find{w}, &Grep{w}, &Bash{w}, &Fetch{},
	}
}

func Select(tools []Tool, allow, deny []string) ([]Tool, error) {
	known := make([]string, len(tools))
	for i, t := range tools {
		known[i] = t.Definition().Name
	}
	for _, name := range slices.Concat(allow, deny) {
		if !slices.Contains(known, name) {
			return nil, fmt.Errorf("no tool named %q — have %s", name, strings.Join(known, ", "))
		}
	}

	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		name := t.Definition().Name
		if len(allow) > 0 && !slices.Contains(allow, name) {
			continue
		}
		if slices.Contains(deny, name) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func Names(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Definition().Name
	}
	return out
}

type Func struct {
	Def    llm.Tool
	Action permission.Action
	Target string
	Do     func(ctx context.Context, input json.RawMessage) (string, error)
}

func (f *Func) Definition() llm.Tool { return f.Def }

func (f *Func) Prepare(input json.RawMessage) (Call, error) {
	return &funcCall{tool: f, input: input}, nil
}

type funcCall struct {
	tool  *Func
	input json.RawMessage
}

func (c *funcCall) Request() permission.Request {
	action := c.tool.Action
	if action == "" {
		action = permission.Execute
	}
	return permission.Request{Tool: c.tool.Def.Name, Action: action, Target: c.tool.Target}
}

func (c *funcCall) Run(ctx context.Context) (string, error) { return c.tool.Do(ctx, c.input) }

func parse[T any](input json.RawMessage) (T, error) {
	var args T
	if len(input) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return args, fmt.Errorf("bad arguments: %w", err)
	}
	return args, nil
}

func object(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func field(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}
