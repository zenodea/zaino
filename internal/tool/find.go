package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Find struct{ w *Workspace }

type findArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (f *Find) Action() permission.Action { return permission.Read }

func (f *Find) Definition() llm.Tool {
	return llm.Tool{
		Name: "find",
		Description: "Find files by name. The pattern is a glob: * matches within one path " +
			"segment, ** matches across segments. A pattern with no slash in it is matched " +
			"at any depth, so \"*.go\" finds every Go file. " +
			"Results are paths relative to the search root, newest first.",
		InputSchema: object(map[string]any{
			"pattern": field("string", "Glob to match, for example \"*.go\" or \"internal/**/*_test.go\"."),
			"path":    field("string", "Directory to search under. Defaults to the working directory."),
		}, "pattern"),
	}
}

func (f *Find) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[findArgs](input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
	}
	root, err := f.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}
	return &findCall{root: root, pattern: args.Pattern}, nil
}

type findCall struct {
	root    Path
	pattern string
}

func (c *findCall) Request() permission.Request {
	return permission.Request{
		Tool:    "find",
		Action:  permission.Read,
		Target:  c.root.String(),
		Preview: c.pattern,
		Outside: c.root.Outside,
	}
}

func (c *findCall) Run(ctx context.Context) (string, error) {
	pattern := c.pattern
	if !strings.Contains(pattern, "/") {
		pattern = "**/" + pattern
	}

	type hit struct {
		path string
		mod  int64
	}
	var hits []hit

	err := filepath.WalkDir(c.root.Abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, relErr := filepath.Rel(c.root.Abs, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !matchGlob(pattern, filepath.ToSlash(rel)) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		hits = append(hits, hit{path: filepath.ToSlash(rel), mod: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return fmt.Sprintf("no files match %s under %s", c.pattern, c.root), nil
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].mod > hits[j].mod })
	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.path
	}
	return clipLines(paths, maxListed, "files"), nil
}

func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := filepath.Match(pattern[0], name[0])
		if err != nil || !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}
