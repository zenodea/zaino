package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/x/fsx"
)

const maxNotes = 200

type Memory struct {
	path string
	mu   sync.Mutex
}

func NewMemory(path string) *Memory { return &Memory{path: path} }

func (m *Memory) Path() string { return m.path }

func (m *Memory) Notes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.read()
}

func (m *Memory) read() []string {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil
	}
	var notes []string
	for _, line := range strings.Split(string(data), "\n") {
		if text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-")); text != "" {
			notes = append(notes, text)
		}
	}
	return notes
}

func (m *Memory) write(notes []string) error {
	if len(notes) == 0 {
		err := os.Remove(m.path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	for _, n := range notes {
		b.WriteString("- " + n + "\n")
	}
	return fsx.WriteAtomic(m.path, []byte(b.String()), 0o600)
}

type memoryArgs struct {
	Action string `json:"action,omitempty"`
	Text   string `json:"text,omitempty"`
}

func (*Memory) Action() permission.Action { return permission.Read }

func (m *Memory) Definition() llm.Tool {
	return llm.Tool{
		Name: "memory",
		Description: "Notes about this project that outlive the session: they are read in at the start of " +
			"every session here. Use add for what a future session would want to know and could not " +
			"find out quickly — conventions, gotchas, where things live, how tests are run, what the " +
			"user prefers — not for progress on the task at hand. Keep each note to one line. " +
			"forget drops the notes containing the text; list shows them all. " +
			"When a note turns out to be wrong, forget it and add the correction.",
		InputSchema: object(map[string]any{
			"action": field("string", "add, forget or list. Defaults to list."),
			"text":   field("string", "The note to add, or the text of the notes to forget."),
		}),
	}
}

func (m *Memory) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[memoryArgs](input)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if action == "" {
		action = "list"
	}
	text := strings.TrimSpace(strings.ReplaceAll(args.Text, "\n", " "))
	switch action {
	case "list":
	case "add", "forget":
		if text == "" {
			return nil, fmt.Errorf("text is required to %s", action)
		}
	default:
		return nil, fmt.Errorf("action must be add, forget or list, not %q", args.Action)
	}
	return &memoryCall{m: m, action: action, text: text}, nil
}

type memoryCall struct {
	m      *Memory
	action string
	text   string
}

func (c *memoryCall) Request() permission.Request {
	return permission.Request{Tool: "memory", Action: permission.Read, Target: c.action, Preview: c.action + " " + c.text}
}

func (c *memoryCall) Run(context.Context) (string, error) {
	c.m.mu.Lock()
	defer c.m.mu.Unlock()
	notes := c.m.read()

	switch c.action {
	case "add":
		for _, n := range notes {
			if strings.EqualFold(n, c.text) {
				return "already noted", nil
			}
		}
		if len(notes) >= maxNotes {
			return "", fmt.Errorf("memory is full at %d notes — forget some first", maxNotes)
		}
		if err := c.m.write(append(notes, c.text)); err != nil {
			return "", err
		}
		return fmt.Sprintf("noted (%d notes)", len(notes)+1), nil

	case "forget":
		kept := notes[:0:0]
		for _, n := range notes {
			if !strings.Contains(strings.ToLower(n), strings.ToLower(c.text)) {
				kept = append(kept, n)
			}
		}
		dropped := len(notes) - len(kept)
		if dropped == 0 {
			return "nothing noted matches that", nil
		}
		if err := c.m.write(kept); err != nil {
			return "", err
		}
		return fmt.Sprintf("forgot %d (%d notes left)", dropped, len(kept)), nil
	}

	if len(notes) == 0 {
		return "(nothing noted yet)", nil
	}
	return "- " + strings.Join(notes, "\n- "), nil
}
