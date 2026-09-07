package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Job struct{}

type jobArgs struct {
	ID     string `json:"id,omitempty"`
	Action string `json:"action,omitempty"`
	Lines  int    `json:"lines,omitempty"`
}

func (*Job) Action() permission.Action { return permission.Read }

func (*Job) Definition() llm.Tool {
	return llm.Tool{
		Name: "job",
		Description: "Look at a command bash started in the background, or stop it. " +
			"output, the default, returns what it has printed so far and whether it is still running; " +
			"kill stops it; list names every job started this session.",
		InputSchema: object(map[string]any{
			"id":     field("string", "The job, as bash named it: j1, j2, …"),
			"action": field("string", "output, kill or list. Defaults to output."),
			"lines":  field("integer", "With output: only the last this many lines."),
		}),
	}
}

func (*Job) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[jobArgs](input)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if action == "" {
		action = "output"
	}
	switch action {
	case "output", "kill", "list":
	default:
		return nil, fmt.Errorf("action must be output, kill or list, not %q", args.Action)
	}
	if action != "list" && strings.TrimSpace(args.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	return &jobCall{id: strings.TrimSpace(args.ID), action: action, lines: args.Lines}, nil
}

type jobCall struct {
	id     string
	action string
	lines  int
}

func (c *jobCall) Request() permission.Request {
	return permission.Request{Tool: "job", Action: permission.Read, Target: c.id, Preview: c.action + " " + c.id}
}

func (c *jobCall) Run(ctx context.Context) (string, error) {
	if c.action == "list" {
		all := jobs.all()
		if len(all) == 0 {
			return "(no jobs started this session)", nil
		}
		lines := make([]string, len(all))
		for i, j := range all {
			lines[i] = fmt.Sprintf("%s  %-22s  %s  %s", j.id, j.status(), time.Since(j.started).Truncate(time.Second), j.command)
		}
		return strings.Join(lines, "\n"), nil
	}

	j, ok := jobs.get(c.id)
	if !ok {
		return "", fmt.Errorf("no job %q — job {\"action\": \"list\"} shows what there is", c.id)
	}
	if c.action == "kill" {
		if !j.running() {
			return fmt.Sprintf("%s had already stopped: %s", j.id, j.status()), nil
		}
		if err := j.kill(); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s stopped: %s", j.id, j.status()), nil
	}

	out, err := j.output(c.lines)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "(no output yet)"
	}
	return fmt.Sprintf("%s · %s · %s\n\n%s", j.id, j.status(), j.command, out), nil
}
