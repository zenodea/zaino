package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

const maxTodos = 50

type TodoItem struct {
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
}

type Todo struct {
	mu    sync.Mutex
	items []TodoItem
}

func NewTodo() *Todo { return &Todo{} }

func (t *Todo) Items() []TodoItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TodoItem(nil), t.items...)
}

func (t *Todo) Progress() (done, total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, it := range t.items {
		if it.Status == "done" {
			done++
		}
	}
	return done, len(t.items)
}

var todoMarks = map[string]string{"pending": "[ ]", "active": "[>]", "done": "[x]"}

func (t *Todo) Render() string {
	items := t.Items()
	if len(items) == 0 {
		return "(no plan laid out)"
	}
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = todoMarks[it.Status] + " " + it.Text
	}
	return strings.Join(lines, "\n")
}

type todoArgs struct {
	Items []TodoItem `json:"items"`
}

func (*Todo) Action() permission.Action { return permission.Read }

func (*Todo) Definition() llm.Tool {
	return llm.Tool{
		Name: "todo",
		Description: "The plan for a task with more than a couple of steps, kept where the user can watch it. " +
			"Each call replaces the whole list. Lay the steps out before starting, mark the one you are " +
			"on active and each one done as you finish it, and add steps as they turn up. " +
			"In plan mode, lay the plan out here and stop: the user looks it over and lets it go.",
		InputSchema: object(map[string]any{
			"items": map[string]any{
				"type":        "array",
				"description": "The steps, in order.",
				"items": object(map[string]any{
					"text":   field("string", "The step, in one line."),
					"status": field("string", "pending, active or done. Defaults to pending."),
				}, "text"),
			},
		}, "items"),
	}
}

func (t *Todo) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[todoArgs](input)
	if err != nil {
		return nil, err
	}
	if len(args.Items) > maxTodos {
		return nil, fmt.Errorf("that is %d steps — keep it under %d", len(args.Items), maxTodos)
	}
	items := make([]TodoItem, 0, len(args.Items))
	for i, it := range args.Items {
		text := strings.TrimSpace(strings.ReplaceAll(it.Text, "\n", " "))
		if text == "" {
			return nil, fmt.Errorf("item %d has no text", i+1)
		}
		status := strings.ToLower(strings.TrimSpace(it.Status))
		if status == "" {
			status = "pending"
		}
		if _, ok := todoMarks[status]; !ok {
			return nil, fmt.Errorf("item %d: status must be pending, active or done, not %q", i+1, it.Status)
		}
		items = append(items, TodoItem{Text: text, Status: status})
	}
	return &todoCall{t: t, items: items}, nil
}

type todoCall struct {
	t     *Todo
	items []TodoItem
}

func (c *todoCall) Request() permission.Request {
	return permission.Request{Tool: "todo", Action: permission.Read, Preview: fmt.Sprintf("%d steps", len(c.items))}
}

func (c *todoCall) Run(context.Context) (string, error) {
	c.t.mu.Lock()
	c.t.items = c.items
	c.t.mu.Unlock()
	done, total := c.t.Progress()
	return fmt.Sprintf("%s\n\n%d of %d done", c.t.Render(), done, total), nil
}
