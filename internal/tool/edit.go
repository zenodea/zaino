package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Edit struct{ w *Workspace }

type editArgs struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (e *Edit) Definition() llm.Tool {
	return llm.Tool{
		Name: "edit",
		Description: "Replace an exact stretch of text in a file. Read the file first. " +
			"old_text must appear in it exactly once, so include enough surrounding lines " +
			"to be unambiguous, or set replace_all to change every occurrence. " +
			"Line numbers from read are not part of the file — do not include them.",
		InputSchema: object(map[string]any{
			"path":        field("string", "File to edit."),
			"old_text":    field("string", "Text to replace, copied exactly from the file."),
			"new_text":    field("string", "Text to put in its place. Empty string deletes it."),
			"replace_all": field("boolean", "Replace every occurrence instead of requiring exactly one."),
		}, "path", "old_text", "new_text"),
	}
}

func (e *Edit) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[editArgs](input)
	if err != nil {
		return nil, err
	}
	if args.OldText == "" {
		return nil, fmt.Errorf("old_text is required — use write to create a file")
	}
	if args.OldText == args.NewText {
		return nil, fmt.Errorf("old_text and new_text are identical")
	}

	path, err := e.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}
	if !e.w.WasRead(path.Abs) {
		return nil, fmt.Errorf("read %s before editing it", path)
	}

	content, err := os.ReadFile(path.Abs)
	if err != nil {
		return nil, pathError(err, path)
	}
	before := string(content)

	spans, how := findSpans(before, args.OldText)
	switch {
	case len(spans) == 0:
		return nil, fmt.Errorf("old_text does not appear in %s", path)
	case len(spans) > 1 && !args.ReplaceAll:
		return nil, fmt.Errorf("old_text appears %d times in %s — add surrounding lines to pick one, or set replace_all",
			len(spans), path)
	}
	if !args.ReplaceAll {
		spans = spans[:1]
	}

	return &editCall{
		w:       e.w,
		path:    path,
		before:  before,
		after:   replaceSpans(before, spans, args.NewText),
		matches: len(spans),
		how:     how,
	}, nil
}

type editCall struct {
	w       *Workspace
	path    Path
	before  string
	after   string
	matches int
	how     string
}

func (c *editCall) Request() permission.Request {
	return permission.Request{
		Tool:    "edit",
		Action:  permission.Write,
		Target:  c.path.String(),
		Preview: diffPreview(c.before, c.after),
		Outside: c.path.Outside,
	}
}

func (c *editCall) Run(context.Context) (string, error) {
	unlock := c.w.Lock(c.path.Abs)
	defer unlock()

	current, err := os.ReadFile(c.path.Abs)
	if err != nil {
		return "", pathError(err, c.path)
	}
	// Between working the edit out and being allowed to make it, someone else
	// may have written the file. Their change is not ours to discard.
	if string(current) != c.before {
		c.w.Forget(c.path.Abs)
		return "", fmt.Errorf("%s changed on disk since it was read — read it again", c.path)
	}

	info, err := os.Stat(c.path.Abs)
	if err != nil {
		return "", pathError(err, c.path)
	}
	if err := os.WriteFile(c.path.Abs, []byte(c.after), info.Mode().Perm()); err != nil {
		return "", err
	}
	c.w.MarkRead(c.path.Abs, []byte(c.after))

	out := fmt.Sprintf("Edited %s", c.path)
	if c.matches > 1 {
		out += fmt.Sprintf(", %d occurrences", c.matches)
	}
	if c.how != "" {
		out += " (" + c.how + ")"
	}
	return out + "\n" + diffPreview(c.before, c.after), nil
}

func replaceSpans(content string, spans []span, with string) string {
	var b strings.Builder
	at := 0
	for _, s := range spans {
		b.WriteString(content[at:s.start])
		b.WriteString(with)
		at = s.end
	}
	b.WriteString(content[at:])
	return b.String()
}
