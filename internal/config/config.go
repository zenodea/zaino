// Package config reads the files zaino is configured with: one set for you,
// under $XDG_CONFIG_HOME/zaino, and one for the project, in a .zaino directory
// beside the code. The project's answer wins where the two disagree, and a
// flag on the command line wins over both.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/x/paths"
)

// File is one config.json as written. The keys are the flag names, so
// -max-tokens is "max-tokens" here, and anything you can pass you can also
// settle on once.
type File struct {
	Provider      string             `json:"provider,omitempty"`
	Model         string             `json:"model,omitempty"`
	MaxTokens     int                `json:"max-tokens,omitempty"`
	Effort        string             `json:"effort,omitempty"`
	Thinking      *bool              `json:"thinking,omitempty"`
	System        string             `json:"system,omitempty"`
	Permission    string             `json:"permission,omitempty"`
	AllowOutside  *bool              `json:"allow-outside,omitempty"`
	Tools         []string           `json:"tools,omitempty"`
	ExcludeTools  []string           `json:"exclude-tools,omitempty"`
	ContextWindow int                `json:"context-window,omitempty"`
	MaxContext    string             `json:"max-context,omitempty"`
	Vim           *bool              `json:"vim,omitempty"`
	Mouse         *bool              `json:"mouse,omitempty"`
	Animate       *bool              `json:"animate,omitempty"`
	Allow         []string           `json:"allow,omitempty"`
	Deny          []string           `json:"deny,omitempty"`
	Profile       string             `json:"profile,omitempty"`
	Profiles      map[string]Profile `json:"profiles,omitempty"`
}

type Profile struct {
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max-tokens,omitempty"`
	Effort    string `json:"effort,omitempty"`
	Thinking  *bool  `json:"thinking,omitempty"`
	System    string `json:"system,omitempty"`
}

type Config struct {
	File

	// The directory holding .zaino, empty when there is none.
	Project string

	// The ZAINO.md files above the working directory, outermost first. Not a
	// setting: it says where zaino is running, so it is read fresh every run
	// rather than recorded with the session.
	Context string

	Commands  []Command
	Subagents []agent.Subagent

	// mcp.json files to connect, outermost first.
	MCP []string

	// Every file that had a say, for /config.
	Sources []string
}

const dirName = ".zaino"

// What -no-config leaves you with: the built-in commands and nothing else.
func None() *Config { return &Config{Commands: Builtins()} }

func Load(cwd string) (*Config, error) {
	cfg := &Config{Project: findProject(cwd), Commands: Builtins()}

	var dirs []string
	if user, err := paths.Config(); err == nil {
		dirs = append(dirs, user)
	}
	if cfg.Project != "" {
		dirs = append(dirs, filepath.Join(cfg.Project, dirName))
	}

	for _, dir := range dirs {
		if err := cfg.read(dir); err != nil {
			return nil, err
		}
	}

	cfg.MCP = mcpFiles(dirs)

	context, from := gatherContext(cwd)
	cfg.Context = context
	cfg.Sources = append(cfg.Sources, from...)
	return cfg, nil
}

func (c *Config) read(dir string) error {
	// system.md and the "system" key are the same setting written two ways,
	// so the file stands in for the key and the key, being the more explicit
	// of the two, overrides it.
	if prompt, ok := readFile(filepath.Join(dir, "system.md")); ok {
		c.File.System = prompt
		c.Sources = append(c.Sources, filepath.Join(dir, "system.md"))
	}

	path := filepath.Join(dir, "config.json")
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return err
	default:
		var file File
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&file); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		c.File.merge(file)
		c.Sources = append(c.Sources, path)
	}

	commands, err := loadCommands(filepath.Join(dir, "commands"))
	if err != nil {
		return err
	}
	c.Commands = mergeCommands(c.Commands, commands)

	subagents, err := loadSubagents(filepath.Join(dir, "agents"))
	if err != nil {
		return err
	}
	c.Subagents = mergeSubagents(c.Subagents, subagents)
	return nil
}

// Scalars overwrite, and the two lists of permission rules add up: a project
// may widen what it is allowed to do without repeating what you already said.
func (f *File) merge(o File) {
	set(&f.Provider, o.Provider)
	set(&f.Model, o.Model)
	set(&f.MaxTokens, o.MaxTokens)
	set(&f.Effort, o.Effort)
	set(&f.System, o.System)
	set(&f.Permission, o.Permission)
	set(&f.ContextWindow, o.ContextWindow)
	set(&f.MaxContext, o.MaxContext)
	set(&f.Profile, o.Profile)
	setBool(&f.Thinking, o.Thinking)
	setBool(&f.AllowOutside, o.AllowOutside)
	setBool(&f.Vim, o.Vim)
	setBool(&f.Mouse, o.Mouse)
	setBool(&f.Animate, o.Animate)

	if len(o.Tools) > 0 {
		f.Tools = o.Tools
	}
	if len(o.ExcludeTools) > 0 {
		f.ExcludeTools = o.ExcludeTools
	}
	f.Allow = append(f.Allow, o.Allow...)
	f.Deny = append(f.Deny, o.Deny...)

	if len(o.Profiles) > 0 && f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	for name, p := range o.Profiles {
		f.Profiles[name] = p
	}
}

func set[T comparable](dst *T, v T) {
	var zero T
	if v != zero {
		*dst = v
	}
}

func setBool(dst **bool, v *bool) {
	if v != nil {
		*dst = v
	}
}

// The project is the nearest directory above the working one holding a .zaino;
// failing that, the repository, so a .zaino at its root is found from anywhere
// inside it.
func findProject(cwd string) string {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}

	repo := ""
	for {
		if isDir(filepath.Join(dir, dirName)) {
			return dir
		}
		if repo == "" && exists(filepath.Join(dir, ".git")) {
			repo = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return repo
		}
		dir = parent
	}
}

// ZAINO.md is read from the repository root down to the working directory, so
// a note in a subdirectory adds to what the root said rather than replacing it.
func gatherContext(cwd string) (string, []string) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", nil
	}

	var dirs []string
	for {
		dirs = append([]string{dir}, dirs...)
		if exists(filepath.Join(dir, ".git")) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	var parts, from []string
	for _, dir := range dirs {
		path := filepath.Join(dir, "ZAINO.md")
		if text, ok := readFile(path); ok {
			parts = append(parts, text)
			from = append(from, path)
		}
	}
	return strings.Join(parts, "\n\n"), from
}

func mcpFiles(dirs []string) []string {
	var out []string
	for i, dir := range dirs {
		path := filepath.Join(dir, "mcp.json")
		if exists(path) {
			out = append(out, path)
			continue
		}
		// mcp.json used to live with the sessions, under the data root.
		if i == 0 {
			if old, err := paths.Data("mcp.json"); err == nil && exists(old) {
				out = append(out, old)
			}
		}
	}
	return out
}

func readFile(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(raw))
	return text, text != ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
