package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A project without its own .zaino is the common case, so the tests that only
// care about the user's config say so by giving one that does not exist.
func setup(t *testing.T) (userDir, project string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	project = filepath.Join(root, "work")
	write(t, filepath.Join(project, ".git", "HEAD"), "ref: refs/heads/main\n")
	return filepath.Join(root, "config", "zaino"), project
}

func TestProjectOverridesUser(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "config.json"), `{"model": "user-model", "effort": "low", "vim": false}`)
	write(t, filepath.Join(project, ".zaino", "config.json"), `{"model": "project-model"}`)

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "project-model" {
		t.Errorf("model = %q, want the project's", cfg.Model)
	}
	if cfg.Effort != "low" {
		t.Errorf("effort = %q, want the user's to survive", cfg.Effort)
	}
	if cfg.Vim == nil || *cfg.Vim {
		t.Error("vim: false did not come through")
	}
	if cfg.Project != project {
		t.Errorf("project = %q, want %q", cfg.Project, project)
	}
}

// Neither layer should have to repeat what the other already allowed.
func TestRulesAddUp(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "config.json"), `{"allow": ["bash:git status"], "deny": ["read:.env"]}`)
	write(t, filepath.Join(project, ".zaino", "config.json"), `{"allow": ["bash:go test"]}`)

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Allow) != 2 {
		t.Errorf("allow = %v, want both", cfg.Allow)
	}
	if len(cfg.Deny) != 1 {
		t.Errorf("deny = %v, want the user's", cfg.Deny)
	}
}

func TestUnknownKeyIsAnError(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "config.json"), `{"modle": "typo"}`)

	_, err := Load(project)
	if err == nil {
		t.Fatal("got nil, want an error naming the file")
	}
	if !strings.Contains(err.Error(), "config.json") {
		t.Errorf("error does not say where: %v", err)
	}
}

func TestSystemFileAndKey(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "system.md"), "be terse\n")

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System != "be terse" {
		t.Errorf("system = %q, want system.md", cfg.System)
	}

	write(t, filepath.Join(user, "config.json"), `{"system": "the key wins"}`)
	if cfg, err = Load(project); err != nil {
		t.Fatal(err)
	}
	if cfg.System != "the key wins" {
		t.Errorf("system = %q, want the config key to override the file", cfg.System)
	}
}

func TestContextIsGatheredOutermostFirst(t *testing.T) {
	_, project := setup(t)
	write(t, filepath.Join(project, "ZAINO.md"), "the repo")
	write(t, filepath.Join(project, "internal", "tool", "ZAINO.md"), "the corner")

	cfg, err := Load(filepath.Join(project, "internal", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "the repo\n\nthe corner"; cfg.Context != want {
		t.Errorf("context = %q, want %q", cfg.Context, want)
	}
	if len(cfg.Sources) != 2 {
		t.Errorf("sources = %v, want both files", cfg.Sources)
	}
}

// The system prompt is a setting and belongs to the session; ZAINO.md
// describes where zaino is running and must not be mistaken for one.
func TestContextIsNotTheSystemPrompt(t *testing.T) {
	_, project := setup(t)
	write(t, filepath.Join(project, "ZAINO.md"), "the repo")

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System != "" {
		t.Errorf("system = %q, want it left alone", cfg.System)
	}
}

func TestProjectFoundFromASubdirectory(t *testing.T) {
	_, project := setup(t)
	write(t, filepath.Join(project, ".zaino", "config.json"), `{"model": "found"}`)

	cfg, err := Load(filepath.Join(project, "internal", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "found" {
		t.Error("the project's config was not found from inside it")
	}
}

func TestCommandsAndAgents(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "commands", "review.md"), "---\ndescription: read the diff\n---\nReview $ARGUMENTS please.")
	write(t, filepath.Join(project, ".zaino", "commands", "review.md"), "the project's version")
	write(t, filepath.Join(user, "agents", "scout.md"), "---\ndescription: find things\ntools: read, grep\n---\nYou search.")

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	review, ok := findCommand(cfg.Commands, "review")
	if !ok {
		t.Fatal("/review was not loaded")
	}
	if review.Prompt != "the project's version" {
		t.Errorf("prompt = %q, want the project's to replace the user's", review.Prompt)
	}
	if len(cfg.Commands) != len(Builtins())+1 {
		t.Errorf("commands = %d, want the built-ins and one more", len(cfg.Commands))
	}
	if len(cfg.Subagents) != 1 {
		t.Fatalf("subagents = %d, want one", len(cfg.Subagents))
	}
	if got := cfg.Subagents[0]; got.Name != "scout" || len(got.Tools) != 2 || got.System != "You search." {
		t.Errorf("subagent = %+v", got)
	}
}

func TestABuiltinCommandCanBeReplaced(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "commands", "bro.md"), "say it in latin")

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Commands) != len(Builtins()) {
		t.Errorf("commands = %d, want the file to replace the built-in", len(cfg.Commands))
	}
	bro, _ := findCommand(cfg.Commands, "bro")
	if bro.Prompt != "say it in latin" {
		t.Errorf("prompt = %q", bro.Prompt)
	}
}

func TestExpand(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		arg    string
		want   string
	}{
		{"everything", "Review $ARGUMENTS please", "the diff", "Review the diff please"},
		{"positional", "compare $1 and $2", "a b", "compare a and b"},
		{"missing positional", "compare $1 and $2", "a", "compare a and "},
		{"unasked for", "Review the diff", "twice", "Review the diff\n\ntwice"},
		{"nothing to add", "Review the diff", "", "Review the diff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Command{Prompt: tt.prompt}).Expand(tt.arg); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMCPFilesIncludeTheOldHome(t *testing.T) {
	_, project := setup(t)
	data := os.Getenv("XDG_DATA_HOME")
	write(t, filepath.Join(data, "zaino", "mcp.json"), `{"servers":{}}`)

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCP) != 1 || !strings.HasPrefix(cfg.MCP[0], data) {
		t.Errorf("mcp = %v, want the file left under the data root", cfg.MCP)
	}

	user := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "zaino")
	write(t, filepath.Join(user, "mcp.json"), `{"servers":{}}`)
	if cfg, err = Load(project); err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCP) != 1 || !strings.HasPrefix(cfg.MCP[0], user) {
		t.Errorf("mcp = %v, want the config root to win", cfg.MCP)
	}
}

func TestNoConfigAnywhere(t *testing.T) {
	_, project := setup(t)

	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "" || cfg.System != "" || len(cfg.Sources) != 0 {
		t.Errorf("got %+v, want an empty config", cfg)
	}
	if len(cfg.Commands) != len(Builtins()) {
		t.Errorf("commands = %d, want the built-ins to be there regardless", len(cfg.Commands))
	}
}
