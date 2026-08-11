package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Ls struct{ w *Workspace }

type lsArgs struct {
	Path string `json:"path,omitempty"`
	All  bool   `json:"all,omitempty"`
}

func (l *Ls) Definition() llm.Tool {
	return llm.Tool{
		Name:        "ls",
		Description: "List the entries of a directory. Directories come back with a trailing slash.",
		InputSchema: object(map[string]any{
			"path": field("string", "Directory to list. Defaults to the working directory."),
			"all":  field("boolean", "Include entries whose name starts with a dot."),
		}),
	}
}

func (l *Ls) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[lsArgs](input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
	}
	path, err := l.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}
	return &lsCall{path: path, all: args.All}, nil
}

type lsCall struct {
	path Path
	all  bool
}

func (c *lsCall) Request() permission.Request {
	return permission.Request{
		Tool:    "ls",
		Action:  permission.Read,
		Target:  c.path.String(),
		Outside: c.path.Outside,
	}
}

func (c *lsCall) Run(context.Context) (string, error) {
	entries, err := os.ReadDir(c.path.Abs)
	if err != nil {
		return "", pathError(err, c.path)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !c.all && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			names = append(names, e.Name()+"/")
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return fmt.Sprintf("%s is empty", c.path), nil
	}
	sort.Strings(names)
	return clipLines(names, maxListed, "entries"), nil
}
