package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Grep struct{ w *Workspace }

type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
}

func (g *Grep) Action() permission.Action { return permission.Read }

func (g *Grep) Definition() llm.Tool {
	return llm.Tool{
		Name: "grep",
		Description: "Search file contents with a regular expression (Go/RE2 syntax). " +
			"Returns matching lines as path:line:text. Binary files and build directories " +
			"are skipped.",
		InputSchema: object(map[string]any{
			"pattern":     field("string", "Regular expression to search for."),
			"path":        field("string", "Directory or file to search. Defaults to the working directory."),
			"glob":        field("string", "Only search files whose path matches this glob, for example \"*.go\"."),
			"ignore_case": field("boolean", "Match without regard to case."),
		}, "pattern"),
	}
}

func (g *Grep) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[grepArgs](input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	expr := args.Pattern
	if args.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("bad pattern: %w", err)
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
	}
	root, err := g.w.Resolve(args.Path)
	if err != nil {
		return nil, err
	}
	return &grepCall{root: root, re: re, pattern: args.Pattern, glob: args.Glob}, nil
}

type grepCall struct {
	root    Path
	re      *regexp.Regexp
	pattern string
	glob    string
}

func (c *grepCall) Request() permission.Request {
	return permission.Request{
		Tool:    "grep",
		Action:  permission.Read,
		Target:  c.root.String(),
		Preview: c.pattern,
		Outside: c.root.Outside,
	}
}

func (c *grepCall) Run(ctx context.Context) (string, error) {
	glob := c.glob
	if glob != "" && !strings.Contains(glob, "/") {
		glob = "**/" + glob
	}

	var found []string
	files := 0

	err := filepath.WalkDir(c.root.Abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(found) >= maxMatches {
			return filepath.SkipAll
		}

		rel, relErr := filepath.Rel(c.root.Abs, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if glob != "" && !matchGlob(glob, rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > maxFileBytes {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil || isBinary(content) {
			return nil
		}
		files++

		for i, line := range strings.Split(string(content), "\n") {
			if !c.re.MatchString(line) {
				continue
			}
			text, _ := clip(strings.TrimRight(line, "\r"), 300)
			found = append(found, fmt.Sprintf("%s:%d:%s", rel, i+1, text))
			if len(found) >= maxMatches {
				break
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return fmt.Sprintf("no matches for %s in %d files", c.pattern, files), nil
	}
	sort.Strings(found)
	return clipLines(found, maxMatches, "matches"), nil
}
