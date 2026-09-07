package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

type changingTool struct{ tool.Func }

type changingCall struct {
	tool.Call
	root string
}

func (c *changingCall) Run(ctx context.Context) (string, error) {
	c.root = tool.RootCallID(ctx)
	return c.Call.Run(ctx)
}

func (c *changingCall) Changes() []tool.Change {
	return []tool.Change{{Path: "a.txt", After: []byte("made")}}
}

func (t *changingTool) Prepare(input json.RawMessage) (tool.Call, error) {
	call, err := t.Func.Prepare(input)
	return &changingCall{Call: call}, err
}

func TestAChangedFileIsReportedUnderItsCall(t *testing.T) {
	ag, _ := newTestAgent(t, oneToolTurn("scribble"), textTurn("done"))
	ag.Tools = []tool.Tool{&changingTool{tool.Func{
		Def:    llm.Tool{Name: "scribble", InputSchema: map[string]any{"type": "object"}},
		Action: permission.Write,
		Do:     func(context.Context, json.RawMessage) (string, error) { return "ok", nil },
	}}}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}

	var gotID string
	var got tool.Change
	ag.Hooks.OnFileChange = func(id string, c tool.Change) { gotID, got = id, c }

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")}); err != nil {
		t.Fatal(err)
	}
	if gotID != "toolu_a" || got.Path != "a.txt" || string(got.After) != "made" {
		t.Errorf("reported %q %+v", gotID, got)
	}
}
