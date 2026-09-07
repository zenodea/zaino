package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenodea/zaino/internal/hook"
	"github.com/zenodea/zaino/internal/store/last"
)

func TestHooksAddUpAcrossTheFiles(t *testing.T) {
	user, project := setup(t)
	write(t, filepath.Join(user, "config.json"),
		`{"hooks": {"pre-tool": [{"tool": "bash, edit", "run": "guard", "timeout_ms": 500}]}}`)
	write(t, filepath.Join(project, ".zaino", "config.json"),
		`{"hooks": {"pre-tool": [{"run": "project-guard"}], "turn-end": [{"run": "ding"}]}}`)

	cfg, err := Load(project, last.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := cfg.HookList()
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 3 {
		t.Fatalf("got %d hooks: %+v", len(hooks), hooks)
	}
	first := hooks[0]
	if first.Event != hook.PreTool || first.Run != "guard" || first.Timeout != 500*time.Millisecond {
		t.Errorf("first = %+v", first)
	}
	if len(first.Tools) != 2 || first.Tools[1] != "edit" {
		t.Errorf("tools = %v, want the comma list split and trimmed", first.Tools)
	}
	if hooks[1].Run != "project-guard" || len(hooks[1].Tools) != 0 {
		t.Errorf("second = %+v, want the project's after the user's", hooks[1])
	}
	if hooks[2].Event != hook.TurnEnd {
		t.Errorf("third = %+v", hooks[2])
	}
}

func TestHooksRefuseWhatTheyCannotRun(t *testing.T) {
	f := File{Hooks: map[string][]HookSpec{"on-tuesday": {{Run: "x"}}}}
	if _, err := f.HookList(); err == nil || !strings.Contains(err.Error(), "on-tuesday") {
		t.Errorf("unknown event: %v", err)
	}
	f = File{Hooks: map[string][]HookSpec{"pre-tool": {{Tool: "bash"}}}}
	if _, err := f.HookList(); err == nil || !strings.Contains(err.Error(), "run is empty") {
		t.Errorf("empty run: %v", err)
	}
}
