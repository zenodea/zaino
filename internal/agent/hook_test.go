package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/hook"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

func TestAPreToolHookCanBlockTheCall(t *testing.T) {
	ag, _ := newTestAgent(t, oneToolTurn("scribble"), textTurn("fine"))
	var ran bool
	var mu sync.Mutex
	ag.Tools = []tool.Tool{writingTool("scribble", &ran, &mu)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}
	ag.Hook = hook.New(t.TempDir(), []hook.Hook{
		{Event: hook.PreTool, Tools: []string{"scribble"}, Run: `echo "not on a friday" >&2; exit 2`},
	})

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("the tool ran past a blocking hook")
	}
	result := lastResult(t, history)
	if !result.IsError || !strings.Contains(result.Content, "not on a friday") {
		t.Errorf("result = %+v, want the hook's reason as an error", result)
	}
}

func TestAPostToolHookSpeaksIntoTheResult(t *testing.T) {
	ag, _ := newTestAgent(t, oneToolTurn("scribble"), textTurn("fine"))
	var ran bool
	var mu sync.Mutex
	ag.Tools = []tool.Tool{writingTool("scribble", &ran, &mu)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}
	var failed []error
	ag.Hooks.OnHookFailed = func(err error) { failed = append(failed, err) }
	ag.Hook = hook.New(t.TempDir(), []hook.Hook{
		{Event: hook.PostTool, Run: `grep -q '"result":"did it"' && echo "looks tidy"`},
		{Event: hook.PostTool, Run: `exit 1`},
		{Event: hook.TurnEnd, Run: `exit 3`},
	})

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatal(err)
	}
	result := lastResult(t, history)
	if result.IsError || !strings.Contains(result.Content, "did it") || !strings.Contains(result.Content, "looks tidy") {
		t.Errorf("result = %+v, want the tool's output and the hook's line", result)
	}
	if len(failed) != 2 {
		t.Errorf("reported %d failures, want the post-tool exit 1 and the turn-end exit 3: %v", len(failed), failed)
	}
}
