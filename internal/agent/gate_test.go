package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

func writingTool(name string, ran *bool, mu *sync.Mutex) tool.Tool {
	return &tool.Func{
		Def:    llm.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
		Action: permission.Write,
		Target: "a.txt",
		Do: func(context.Context, json.RawMessage) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			*ran = true
			return "did it", nil
		},
	}
}

func oneToolTurn(name string) string {
	return sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"`+name+`","input":{}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)
}

func lastResult(t *testing.T, history []llm.Message) llm.ToolResultBlock {
	t.Helper()
	for i := len(history) - 1; i >= 0; i-- {
		for _, block := range history[i].Content {
			if result, ok := block.(llm.ToolResultBlock); ok {
				return result
			}
		}
	}
	t.Fatal("no tool result in history")
	return llm.ToolResultBlock{}
}

func TestGateStopsTheToolRunning(t *testing.T) {
	tests := []struct {
		name    string
		mode    permission.Mode
		wantRan bool
		wantErr string
	}{
		{"plan mode refuses", permission.Plan, false, "plan mode is read only"},
		{"bypass lets it through", permission.Bypass, true, ""},
		{"accept-edits lets a write through", permission.AcceptEdits, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			ran := false

			ag, _ := newTestAgent(t, oneToolTurn("touch"), textTurn("done"))
			ag.Tools = []tool.Tool{writingTool("touch", &ran, &mu)}
			ag.Gate = &permission.Gate{Policy: permission.NewPolicy(tt.mode)}

			history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if ran != tt.wantRan {
				t.Errorf("tool ran = %v, want %v", ran, tt.wantRan)
			}

			result := lastResult(t, history)
			if tt.wantErr == "" {
				if result.IsError {
					t.Errorf("result = %q, want success", result.Content)
				}
				return
			}
			if !result.IsError {
				t.Errorf("result = %q, want an error the model can read", result.Content)
			}
			if !strings.Contains(result.Content, tt.wantErr) {
				t.Errorf("result = %q, want it to mention %q", result.Content, tt.wantErr)
			}
		})
	}
}

func TestRefusalReachesTheModelAsAResult(t *testing.T) {
	var mu sync.Mutex
	ran := false

	ag, _ := newTestAgent(t, oneToolTurn("touch"), textTurn("fair enough"))
	ag.Tools = []tool.Tool{writingTool("touch", &ran, &mu)}
	ag.Gate = &permission.Gate{
		Policy:   permission.NewPolicy(permission.Manual),
		Approver: &fakeApprover{grant: permission.Reject},
	}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if ran {
		t.Error("tool ran despite being refused")
	}
	if result := lastResult(t, history); !strings.Contains(result.Content, "you said no") {
		t.Errorf("result = %q, want the refusal explained", result.Content)
	}
	if history[len(history)-1].Text() != "fair enough" {
		t.Errorf("last message = %q, want the loop to have continued", history[len(history)-1].Text())
	}
}

type fakeApprover struct {
	mu    sync.Mutex
	grant permission.Grant
	seen  []permission.Request
}

func (f *fakeApprover) Approve(_ context.Context, req permission.Request) (permission.Grant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, req)
	return f.grant, nil
}

func TestApprovalsAreAskedOneAtATime(t *testing.T) {
	toolTurn := sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"touch","input":{}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_b","name":"touch","input":{}}}`,
		`data: {"type":"content_block_stop","index":1}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)

	var mu sync.Mutex
	ran := false
	approver := &fakeApprover{grant: permission.Once}

	ag, _ := newTestAgent(t, toolTurn, textTurn("done"))
	ag.Tools = []tool.Tool{writingTool("touch", &ran, &mu)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Manual), Approver: approver}

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	approver.mu.Lock()
	defer approver.mu.Unlock()
	if len(approver.seen) != 2 {
		t.Fatalf("asked %d times, want 2", len(approver.seen))
	}
	for _, req := range approver.seen {
		if req.Action != permission.Write || req.Target != "a.txt" {
			t.Errorf("request = %+v, want the tool's own declared action and target", req)
		}
	}
}

func TestUnknownToolIsAnErrorNotACrash(t *testing.T) {
	ag, _ := newTestAgent(t, oneToolTurn("nonesuch"), textTurn("ok"))
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result := lastResult(t, history); !strings.Contains(result.Content, "no tool named") {
		t.Errorf("result = %q", result.Content)
	}
}
