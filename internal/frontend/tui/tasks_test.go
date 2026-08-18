package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func taskUse(id, what string) llm.ToolUseBlock {
	input, _ := json.Marshal(map[string]string{"description": what, "prompt": "go and " + what})
	return llm.ToolUseBlock{ID: id, Name: "task", Input: input}
}

func spawn(m *Model, id, what string) llm.ToolUseBlock {
	call := taskUse(id, what)
	send(m,
		toolCallMsg{Call: call},
		taskStartMsg{info: agent.TaskInfo{ID: id, Description: what, Model: "test-model", Depth: 1}},
	)
	return call
}

func TestChildEventsStayOutOfTheMainTranscript(t *testing.T) {
	m := newTestModel(t, 100, 24)
	spawn(m, "toolu_t", "scout around")

	grep := llm.ToolUseBlock{ID: "c1", Name: "grep", Input: json.RawMessage(`{"pattern":"needle"}`)}
	send(m,
		taskMsg{id: "toolu_t", msg: toolCallMsg{Call: grep}},
		taskMsg{id: "toolu_t", msg: toolResultMsg{Call: grep, Result: "found it"}},
		taskMsg{id: "toolu_t", msg: textDeltaMsg("one match, in main.go")},
	)

	if len(m.entries) != 1 {
		t.Fatalf("main transcript has %d entries, want only the task card: %+v", len(m.entries), m.entries)
	}
	if card := m.rendered[0]; !strings.Contains(card, "scout around") || !strings.Contains(card, "1 tool") {
		t.Errorf("card = %q, want the description and a tool tally", card)
	}

	child := m.taskIndex["toolu_t"]
	if child == nil {
		t.Fatal("the task was not registered")
	}
	if len(child.entries) != 2 || child.entries[0].kind != entryTool || child.entries[1].kind != entryAssistant {
		t.Fatalf("child transcript = %+v, want its call and its answer", child.entries)
	}
	if !child.entries[0].done || child.entries[0].resultLen != len("found it") {
		t.Errorf("child call = %+v, want it completed in place", child.entries[0])
	}
}

func TestTaskCardSettlesWhenTheChildFinishes(t *testing.T) {
	m := newTestModel(t, 100, 24)
	call := spawn(m, "toolu_t", "count the files")

	send(m,
		taskMsg{id: "toolu_t", msg: turnMsg{Model: "test-model", Usage: llm.Usage{InputTokens: 50, OutputTokens: 2000}}},
		taskDoneMsg{id: "toolu_t", history: []llm.Message{llm.UserText("count")}},
		toolResultMsg{Call: call, Result: "forty-two"},
	)

	child := m.taskIndex["toolu_t"]
	if !child.done || child.failed {
		t.Fatalf("task = %+v, want it finished cleanly", child)
	}
	if !m.entries[0].done {
		t.Error("the card is still open")
	}
	if note := m.entries[0].taskNote; !strings.Contains(note, "2.0k↓") {
		t.Errorf("note = %q, want the child's spend in the tally", note)
	}
	if m.sessionUsage.OutputTokens != 2000 {
		t.Errorf("session usage = %+v, want the child's turns counted", m.sessionUsage)
	}
}

func TestParallelResultsFindTheirOwnCalls(t *testing.T) {
	m := newTestModel(t, 80, 24)
	first := llm.ToolUseBlock{ID: "a", Name: "grep", Input: json.RawMessage(`{"pattern":"one"}`)}
	second := llm.ToolUseBlock{ID: "b", Name: "grep", Input: json.RawMessage(`{"pattern":"two"}`)}

	send(m,
		toolCallMsg{Call: first},
		toolCallMsg{Call: second},
		toolResultMsg{Call: second, Result: "second done"},
	)

	if m.entries[0].done {
		t.Error("the first call took the second's result")
	}
	if !m.entries[1].done || m.entries[1].toolResult != "second done" {
		t.Errorf("second call = %+v, want it closed by its own result", m.entries[1])
	}
}

func TestAgentsBoardWalksIntoAChild(t *testing.T) {
	m := newTestModel(t, 100, 24)
	spawn(m, "toolu_t", "scout around")
	send(m, taskMsg{id: "toolu_t", msg: textDeltaMsg("what I found: nothing")})

	m.runCommand("/agents")
	if !m.agents.open || m.agents.viewing != -1 {
		t.Fatalf("board = %+v, want it open on the list", m.agents)
	}
	if view := m.agentsView(); !strings.Contains(view, "scout around") {
		t.Errorf("list = %q, want the task on it", view)
	}

	m.Update(key("enter"))
	if m.agents.viewing != 0 {
		t.Fatalf("viewing = %d, want the child's transcript", m.agents.viewing)
	}
	if view := m.agentsView(); !strings.Contains(view, "what I found: nothing") {
		t.Errorf("transcript view = %q, want the child's words", view)
	}

	m.Update(key("q"))
	if m.agents.viewing != -1 {
		t.Fatal("q did not step back to the list")
	}
	m.Update(key("q"))
	if m.agents.open {
		t.Fatal("q did not close the board")
	}
}

func TestBoardWithNothingSpawned(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.runCommand("/agents")
	if m.agents.open {
		t.Fatal("the board opened with nothing on it")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "no agents") {
		t.Errorf("entry = %+v", last)
	}
}

func TestEnterOnTheCardWalksIn(t *testing.T) {
	m := newTestModel(t, 100, 24)
	spawn(m, "toolu_t", "scout around")

	m.Update(key("ctrl+j"))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want it on the card", m.cursor)
	}
	m.Update(key("enter"))

	if !m.agents.open || m.agents.viewing != 0 {
		t.Fatalf("board = %+v, want the child's transcript on screen", m.agents)
	}
}

func TestRestoreBringsTheTasksBack(t *testing.T) {
	m := newTestModel(t, 100, 24)

	m.Restore(session.Context{
		Messages: []llm.Message{
			llm.UserText("survey the code"),
			{Role: llm.RoleAssistant, Content: llm.Content{taskUse("toolu_t", "scout around")}},
			{Role: llm.RoleUser, Content: llm.Content{
				llm.ToolResultBlock{ToolUseID: "toolu_t", Content: "nothing to report"},
			}},
			{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "All quiet."}}},
		},
		Tasks: []session.TaskBody{{
			ID:          "toolu_t",
			Description: "scout around",
			Model:       "test-model",
			Depth:       1,
			Messages: []llm.Message{
				llm.UserText("go and scout around"),
				{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "nothing to report"}}},
			},
			Usage: llm.Usage{InputTokens: 30, OutputTokens: 7},
		}},
	})

	child := m.taskIndex["toolu_t"]
	if child == nil || !child.done {
		t.Fatalf("task = %+v, want it back and settled", child)
	}
	if len(child.entries) != 2 {
		t.Fatalf("child transcript = %+v, want its two messages", child.entries)
	}
	if note := m.entries[1].taskNote; note == "" {
		t.Error("the restored card has no tally")
	}

	m.runCommand("/agents")
	m.Update(key("enter"))
	if view := m.agentsView(); !strings.Contains(view, "nothing to report") {
		t.Errorf("transcript view = %q, want the child's words back", view)
	}
}

func TestStopOneChildFromTheBoard(t *testing.T) {
	m := newTestModel(t, 100, 24)
	cancelled := false
	call := taskUse("toolu_t", "dig forever")
	send(m, toolCallMsg{Call: call}, taskStartMsg{info: agent.TaskInfo{
		ID: "toolu_t", Description: "dig forever", Depth: 1,
		Cancel: func() { cancelled = true },
	}})

	m.runCommand("/agents")
	m.Update(key("x"))

	if !cancelled {
		t.Fatal("x did not reach the child's cancel")
	}
}
