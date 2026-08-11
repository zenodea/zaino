package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Write struct{ w *Workspace }

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *Write) Definition() llm.Tool {
	return llm.Tool{
		Name: "write",
		Description: "Write a file, replacing whatever is there. Read an existing file before " +
			"overwriting it. Prefer edit for changes to a file that already exists; " +
			"this is for new ones.",
		InputSchema: object(map[string]any{
			"path":    field("string", "File to write."),
			"content": field("string", "The complete new contents of the file."),
		}, "path", "content"),
	}
}

func (w *Write) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[writeArgs](input)
	if err != nil {
		return nil, err
	}
	path, err := w.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}

	call := &writeCall{w: w.w, path: path, after: args.Content}

	existing, err := os.ReadFile(path.Abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		call.creating = true
	case err != nil:
		return nil, pathError(err, path)
	default:
		if !w.w.WasRead(path.Abs) {
			return nil, fmt.Errorf("%s already exists — read it before overwriting it", path)
		}
		call.before = string(existing)
	}
	return call, nil
}

type writeCall struct {
	w        *Workspace
	path     Path
	before   string
	after    string
	creating bool
}

func (c *writeCall) Request() permission.Request {
	preview := diffPreview(c.before, c.after)
	if c.creating {
		preview = fmt.Sprintf("new file, %d lines\n%s", len(splitLines(c.after)), preview)
	}
	return permission.Request{
		Tool:    "write",
		Action:  permission.Write,
		Target:  c.path.String(),
		Preview: preview,
		Outside: c.path.Outside,
	}
}

func (c *writeCall) Run(context.Context) (string, error) {
	unlock := c.w.Lock(c.path.Abs)
	defer unlock()

	current, err := os.ReadFile(c.path.Abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !c.creating {
			return "", fmt.Errorf("%s was deleted since it was read", c.path)
		}
	case err != nil:
		return "", pathError(err, c.path)
	default:
		if c.creating {
			return "", fmt.Errorf("%s was created since this write was worked out — read it first", c.path)
		}
		if string(current) != c.before {
			c.w.Forget(c.path.Abs)
			return "", fmt.Errorf("%s changed on disk since it was read — read it again", c.path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(c.path.Abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(c.path.Abs, []byte(c.after), 0o644); err != nil {
		return "", err
	}
	c.w.MarkRead(c.path.Abs, []byte(c.after))

	if c.creating {
		return fmt.Sprintf("Created %s, %d lines", c.path, len(splitLines(c.after))), nil
	}
	return fmt.Sprintf("Wrote %s\n%s", c.path, diffPreview(c.before, c.after)), nil
}
