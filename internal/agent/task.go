package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

const (
	DefaultTaskTurns = 16
	maxTaskDepth     = 2
)

// Task runs a nested loop with its own context and hands back only what it
// concluded. The point is the context: a search that reads twenty files costs
// the parent one paragraph instead of twenty file contents.
type Task struct {
	parent *Agent
	depth  int
}

func TaskTool(parent *Agent) tool.Tool { return &Task{parent: parent} }

type taskArgs struct {
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

func (t *Task) Action() permission.Action { return permission.Read }

func (t *Task) Definition() llm.Tool {
	return llm.Tool{
		Name: "task",
		Description: "Hand a self-contained piece of work to a second agent and get back only " +
			"its answer. It has the same tools but its own conversation, so use it when finding " +
			"something out would otherwise mean reading a lot into this one — searching a " +
			"codebase, checking a claim across many files, surveying how something is used. " +
			"It cannot ask you anything, so say everything it needs in the prompt, and tell it " +
			"what to report back.",
		InputSchema: object(map[string]any{
			"description": field("string", "A few words naming the job, for the transcript."),
			"prompt":      field("string", "Everything the second agent needs, and what to report back."),
		}, "description", "prompt"),
	}
}

func (t *Task) Prepare(input json.RawMessage) (tool.Call, error) {
	args, err := parse[taskArgs](input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if t.depth >= maxTaskDepth {
		return nil, fmt.Errorf("already %d agents deep — do this one yourself", t.depth)
	}

	what := strings.TrimSpace(args.Description)
	if what == "" {
		what = "task"
	}
	return &taskCall{parent: t.parent, depth: t.depth, what: what, prompt: args.Prompt}, nil
}

type taskCall struct {
	parent *Agent
	depth  int
	what   string
	prompt string
}

// The gate the child inherits is the parent's, so a subagent cannot be used to
// get around a refusal: it asks with the same policy and the same approver.
func (c *taskCall) Request() permission.Request {
	return permission.Request{
		Tool:    "task",
		Action:  permission.Read,
		Target:  c.what,
		Preview: c.prompt,
	}
}

func (c *taskCall) Run(ctx context.Context) (string, error) {
	child := &Agent{
		Provider:  c.parent.Provider,
		Model:     c.parent.Model,
		MaxTokens: c.parent.MaxTokens,
		System:    c.parent.System,
		Effort:    c.parent.Effort,
		Thinking:  c.parent.Thinking,
		Gate:      c.parent.Gate,
		MaxTurns:  orDefault(c.parent.TaskTurns, DefaultTaskTurns),
		Hooks:     Hooks{OnToolCall: c.parent.Hooks.OnToolCall, OnToolResult: c.parent.Hooks.OnToolResult},
	}
	child.Tools = deepen(c.parent.Tools, child, c.depth+1)

	history, err := child.Run(ctx, []llm.Message{llm.UserText(c.prompt)})
	if err != nil {
		return "", fmt.Errorf("%s: %w", c.what, err)
	}

	answer := strings.TrimSpace(lastText(history))
	if answer == "" {
		return "", fmt.Errorf("%s finished without saying anything", c.what)
	}
	return answer, nil
}

// The child gets the same tools, except that its own task tool knows how deep
// it already is.
func deepen(tools []tool.Tool, parent *Agent, depth int) []tool.Tool {
	out := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if _, ok := t.(*Task); ok {
			out = append(out, &Task{parent: parent, depth: depth})
			continue
		}
		out = append(out, t)
	}
	return out
}

func lastText(history []llm.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != llm.RoleAssistant {
			continue
		}
		if text := history[i].Text(); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

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
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func field(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}
