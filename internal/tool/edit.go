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
	OldText    string `json:"old_text,omitempty"`
	NewText    string `json:"new_text,omitempty"`
	ReplaceAll bool   `json:"replace_all,omitempty"`

	Edits []editStep `json:"edits,omitempty"`
}

type editStep struct {
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (a editArgs) steps() []editStep {
	if len(a.Edits) > 0 {
		return a.Edits
	}
	return []editStep{{OldText: a.OldText, NewText: a.NewText, ReplaceAll: a.ReplaceAll}}
}

func (e *Edit) Action() permission.Action { return permission.Write }

func (e *Edit) Definition() llm.Tool {
	return llm.Tool{
		Name: "edit",
		Description: "Replace exact stretches of text in a file. Read the file first. " +
			"Each old_text must appear exactly once, so include enough surrounding lines " +
			"to be unambiguous, or set replace_all to change every occurrence. " +
			"Line numbers from read are not part of the file — do not include them.\n" +
			"Pass edits to make several changes to the same file in one call: they are " +
			"applied in order, each seeing the result of the last, and if any of them does " +
			"not match, none of them are written.",
		InputSchema: object(map[string]any{
			"path":        field("string", "File to edit."),
			"old_text":    field("string", "Text to replace, copied exactly from the file."),
			"new_text":    field("string", "Text to put in its place. Empty string deletes it."),
			"replace_all": field("boolean", "Replace every occurrence instead of requiring exactly one."),
			"edits": map[string]any{
				"type":        "array",
				"description": "Several edits to the same file, applied in order.",
				"items": object(map[string]any{
					"old_text":    field("string", "Text to replace, copied exactly from the file."),
					"new_text":    field("string", "Text to put in its place."),
					"replace_all": field("boolean", "Replace every occurrence."),
				}, "old_text", "new_text"),
			},
		}, "path"),
	}
}

func (e *Edit) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[editArgs](input)
	if err != nil {
		return nil, err
	}
	steps := args.steps()
	if len(steps) == 1 && steps[0].OldText == "" {
		return nil, fmt.Errorf("old_text is required — use write to create a file")
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

	after, matches, how, err := applySteps(before, steps, path)
	if err != nil {
		return nil, err
	}
	if after == before {
		return nil, fmt.Errorf("that would leave %s unchanged", path)
	}

	return &editCall{
		w:       e.w,
		path:    path,
		before:  before,
		after:   after,
		matches: matches,
		steps:   len(steps),
		how:     how,
	}, nil
}

// Every step has to match before any of them is written: a batch that fails
// half way through would leave the file in a state nobody asked for.
func applySteps(before string, steps []editStep, path Path) (after string, matches int, how string, err error) {
	after = before
	for i, step := range steps {
		where := ""
		if len(steps) > 1 {
			where = fmt.Sprintf(" (edit %d of %d)", i+1, len(steps))
		}
		if step.OldText == "" {
			return "", 0, "", fmt.Errorf("old_text is empty%s", where)
		}
		if step.OldText == step.NewText {
			return "", 0, "", fmt.Errorf("old_text and new_text are identical%s", where)
		}

		spans, fuzz := findSpans(after, step.OldText)
		switch {
		case len(spans) == 0:
			return "", 0, "", fmt.Errorf("old_text does not appear in %s%s", path, where)
		case len(spans) > 1 && !step.ReplaceAll:
			return "", 0, "", fmt.Errorf("old_text appears %d times in %s%s — add surrounding lines to pick one, or set replace_all",
				len(spans), path, where)
		}
		if !step.ReplaceAll {
			spans = spans[:1]
		}
		if fuzz != "" {
			how = fuzz
		}
		matches += len(spans)
		after = replaceSpans(after, spans, step.NewText)
	}
	return after, matches, how, nil
}

type editCall struct {
	w       *Workspace
	path    Path
	before  string
	after   string
	matches int
	steps   int
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
	switch {
	case c.steps > 1:
		out += fmt.Sprintf(", %d edits", c.steps)
	case c.matches > 1:
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
