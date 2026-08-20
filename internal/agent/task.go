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

// TaskInfo describes a spawned child. ID is the task call's own tool-use ID;
// Cancel stops this child without touching the turn.
type TaskInfo struct {
	ID          string
	Description string
	Agent       string
	Model       string
	Depth       int
	Background  bool
	Cancel      context.CancelFunc
}

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
	Agent       string `json:"agent,omitempty"`
	Background  bool   `json:"background,omitempty"`
}

func (t *Task) Action() permission.Action { return permission.Read }

func (t *Task) Definition() llm.Tool {
	description := "Hand a self-contained piece of work to a second agent and get back only " +
		"its answer. It has the same tools but its own conversation, so use it when finding " +
		"something out would otherwise mean reading a lot into this one — searching a " +
		"codebase, checking a claim across many files, surveying how something is used. " +
		"It cannot ask you anything, so say everything it needs in the prompt, and tell it " +
		"what to report back. By default the call waits for the answer. Set background " +
		"when the job is long or you have other work meanwhile: the call returns at once " +
		"and the report arrives as a message when it is done — with your next tool results " +
		"if you are still working, or as a new turn if you had finished."

	properties := map[string]any{
		"description": field("string", "A few words naming the job, for the transcript."),
		"prompt":      field("string", "Everything the second agent needs, and what to report back."),
		"background":  field("boolean", "Return at once and have the report delivered later as a message."),
	}

	if named := t.parent.Subagents; len(named) > 0 {
		lines := make([]string, len(named))
		for i, s := range named {
			lines[i] = s.Name + ": " + orDefault(s.Description, "no description given")
		}
		description += " Named agents are set up for particular work — " +
			strings.Join(lines, "; ") + ". Pick one when it fits; leave it out for a general agent."

		agent := field("string", "Which named agent to use. "+strings.Join(lines, "; "))
		agent["enum"] = t.parent.subagentNames()
		properties["agent"] = agent
	}

	return llm.Tool{
		Name:        "task",
		Description: description,
		InputSchema: object(properties, "description", "prompt"),
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
	// A report has to land somewhere that is still listening; only the main
	// conversation is.
	if args.Background && t.depth > 0 {
		return nil, fmt.Errorf("background agents can only be started from the main conversation")
	}

	var named Subagent
	if args.Agent != "" {
		var err error
		if named, err = t.parent.subagent(args.Agent); err != nil {
			return nil, err
		}
		if _, err := named.toolbox(t.parent.Tools); err != nil {
			return nil, fmt.Errorf("agent %s: %w", named.Name, err)
		}
	}

	what := strings.TrimSpace(args.Description)
	if what == "" {
		what = orDefault(named.Name, "task")
	}
	return &taskCall{parent: t.parent, depth: t.depth, what: what, prompt: args.Prompt,
		agent: named, background: args.Background}, nil
}

type taskCall struct {
	parent     *Agent
	depth      int
	what       string
	prompt     string
	agent      Subagent
	background bool
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
	id := tool.CallID(ctx)

	// A background child outlives the turn that started it, so it is cut
	// from the turn's context; its own cancel is what /agents stops it with.
	if c.background {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	child, info, err := c.spawn(id, cancel)
	if err != nil {
		cancel()
		return "", err
	}

	if !c.background {
		defer cancel()
		return c.await(ctx, child, info)
	}

	go func() {
		defer cancel()
		answer, err := c.await(ctx, child, info)
		c.parent.Steer(llm.UserText(report(info, answer, err)))
	}()
	return fmt.Sprintf("Started %q in the background as agent %s. Its report will arrive "+
		"as a message when it is done; carry on meanwhile.", c.what, id), nil
}

func (c *taskCall) spawn(id string, cancel context.CancelFunc) (*Agent, TaskInfo, error) {
	child := &Agent{
		Provider:  c.parent.Provider,
		Model:     orDefault(c.agent.Model, c.parent.Model),
		MaxTokens: c.parent.MaxTokens,
		System:    orDefault(c.agent.System, c.parent.System),
		Project:   c.parent.Project,
		Subagents: c.parent.Subagents,
		Effort:    c.parent.Effort,
		Thinking:  c.parent.Thinking,
		Gate:      c.parent.Gate,
		MaxTurns:  orDefault(c.parent.TaskTurns, DefaultTaskTurns),
	}

	info := TaskInfo{
		ID:          id,
		Description: c.what,
		Agent:       c.agent.Name,
		Model:       child.Model,
		Depth:       c.depth + 1,
		Background:  c.background,
		Cancel:      cancel,
	}
	if on := c.parent.Hooks.OnTask; on != nil {
		child.Hooks = on(info)
	}

	tools, err := c.agent.toolbox(c.parent.Tools)
	if err != nil {
		return nil, info, fmt.Errorf("%s: %w", c.what, err)
	}
	child.Tools = deepen(tools, child, c.depth+1)
	return child, info, nil
}

func (c *taskCall) await(ctx context.Context, child *Agent, info TaskInfo) (string, error) {
	history, err := child.Run(ctx, []llm.Message{llm.UserText(c.prompt)})
	if done := c.parent.Hooks.OnTaskDone; done != nil {
		done(info.ID, history, err)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", c.what, err)
	}

	answer := strings.TrimSpace(lastText(history))
	if answer == "" {
		return "", fmt.Errorf("%s finished without saying anything", c.what)
	}
	return answer, nil
}

// What a background child's report looks like when it lands in the
// conversation: named, so the model knows which job it is hearing back from.
func report(info TaskInfo, answer string, err error) string {
	if err != nil {
		return fmt.Sprintf("Background agent %s (%q) failed: %v", info.ID, info.Description, err)
	}
	return fmt.Sprintf("Background agent %s (%q) reported back:\n\n%s", info.ID, info.Description, answer)
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
