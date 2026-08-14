package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Read struct{ w *Workspace }

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (r *Read) Action() permission.Action { return permission.Read }

func (r *Read) Definition() llm.Tool {
	return llm.Tool{
		Name: "read",
		Description: "Read a file from disk. Output is line-numbered, which is how you " +
			"refer to places in it. Long files come back truncated; use offset and limit " +
			"to page through the rest.",
		InputSchema: object(map[string]any{
			"path":   field("string", "File to read, absolute or relative to the working directory."),
			"offset": field("integer", "First line to read, 1-based. Defaults to the start of the file."),
			"limit":  field("integer", fmt.Sprintf("How many lines to read. Defaults to %d.", maxReadLines)),
		}, "path"),
	}
}

func (r *Read) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[readArgs](input)
	if err != nil {
		return nil, err
	}
	path, err := r.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}
	return &readCall{w: r.w, path: path, offset: args.Offset, limit: args.Limit}, nil
}

type readCall struct {
	w      *Workspace
	path   Path
	offset int
	limit  int
}

func (c *readCall) Request() permission.Request {
	return permission.Request{
		Tool:    "read",
		Action:  permission.Read,
		Target:  c.path.String(),
		Outside: c.path.Outside,
	}
}

func (c *readCall) Run(context.Context) (string, error) {
	info, err := os.Stat(c.path.Abs)
	if err != nil {
		return "", pathError(err, c.path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory — use ls", c.path)
	}

	content, err := os.ReadFile(c.path.Abs)
	if err != nil {
		return "", pathError(err, c.path)
	}
	if isBinary(content) {
		return "", fmt.Errorf("%s looks like a binary file (%d bytes)", c.path, len(content))
	}
	c.w.MarkRead(c.path.Abs, content)

	if len(content) == 0 {
		return fmt.Sprintf("%s is empty", c.path), nil
	}

	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	start := max(c.offset-1, 0)
	if start >= len(lines) {
		return "", fmt.Errorf("%s has %d lines, offset %d is past the end", c.path, len(lines), c.offset)
	}
	limit := c.limit
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}
	end := min(start+limit, len(lines))

	var b strings.Builder
	for i := start; i < end; i++ {
		line, truncated := clip(lines[i], maxLineRunes)
		if truncated {
			line += " …"
		}
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, line)
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "… %d more lines, read again with offset %d\n", len(lines)-end, end+1)
	}
	return b.String(), nil
}

func pathError(err error, p Path) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s does not exist", p)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%s is not readable", p)
	}
	return err
}
