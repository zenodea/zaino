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

func taskTurn(prompt string) string {
	input, _ := json.Marshal(map[string]string{"description": "look", "prompt": prompt})
	return sseTurn(
		turnStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_t","name":"task","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+
			string(mustJSON(string(input)))+`}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":9}}`,
		turnStop,
	)
}

func mustJSON(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

func TestTaskRunsANestedLoopAndReturnsItsAnswer(t *testing.T) {
	ag, _ := newTestAgent(t,
		taskTurn("find every caller of runTools"),
		textTurn("agent.go:79 is the only caller"),
		textTurn("There is one caller."),
	)
	ag.Tools = []tool.Tool{TaskTool(ag)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result := lastResult(t, history)
	if result.IsError {
		t.Fatalf("task failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "only caller") {
		t.Errorf("result = %q, want the child's answer", result.Content)
	}
}

// The child's own reading must not land in the parent's history; that is the
// whole reason to spawn one.
func TestTaskKeepsItsWorkOutOfTheParent(t *testing.T) {
	ag, _ := newTestAgent(t,
		taskTurn("read everything"),
		textTurn("a very long thing the child read and summarised"),
		textTurn("done"),
	)
	ag.Tools = []tool.Tool{TaskTool(ag)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Bypass)}

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(history) != 4 {
		t.Errorf("parent history has %d messages, want 4 — the child's turns leaked in", len(history))
	}
}

func TestTaskRefusesToNestForever(t *testing.T) {
	ag, _ := newTestAgent(t, textTurn("ok"))
	deep := &Task{parent: ag, depth: maxTaskDepth}

	input, _ := json.Marshal(taskArgs{Description: "again", Prompt: "and again"})
	if _, err := deep.Prepare(input); err == nil || !strings.Contains(err.Error(), "agents deep") {
		t.Errorf("err = %v, want a refusal to nest further", err)
	}
}

func TestTaskNeedsAPrompt(t *testing.T) {
	ag, _ := newTestAgent(t, textTurn("ok"))
	input, _ := json.Marshal(taskArgs{Description: "empty"})

	if _, err := TaskTool(ag).Prepare(input); err == nil {
		t.Error("a task with no prompt was accepted")
	}
}

// A subagent must not be a way around a refusal.
func TestTaskInheritsTheGate(t *testing.T) {
	var mu sync.Mutex
	ran := false

	ag, _ := newTestAgent(t, taskTurn("touch the file"), oneToolTurn("touch"), textTurn("refused"), textTurn("ok"))
	ag.Tools = []tool.Tool{TaskTool(ag), writingTool("touch", &ran, &mu)}
	ag.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Plan)}

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("go")}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if ran {
		t.Error("the child wrote something the parent's policy forbids")
	}
}
