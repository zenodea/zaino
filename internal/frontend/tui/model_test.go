package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
)

func newTestModel(t *testing.T, width, height int) *Model {
	t.Helper()
	ag := &agent.Agent{Provider: stubProvider{}, Model: "claude-opus-5", Effort: llm.EffortXHigh}
	m := New(ag, "anthropic")
	m.resize(width, height)
	return m
}

func send(m *Model, msgs ...tea.Msg) {
	for _, msg := range msgs {
		m.Update(msg)
	}
}

func TestStreamingAppendsToSingleEntry(t *testing.T) {
	m := newTestModel(t, 80, 24)

	send(m,
		textDeltaMsg("Hello"),
		textDeltaMsg(", "),
		textDeltaMsg("world."),
	)

	if len(m.entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(m.entries), m.entries)
	}
	if m.entries[0].kind != entryAssistant {
		t.Errorf("kind = %v, want assistant", m.entries[0].kind)
	}
	if m.entries[0].text != "Hello, world." {
		t.Errorf("text = %q", m.entries[0].text)
	}
}

func TestThinkingAndTextSplit(t *testing.T) {
	m := newTestModel(t, 80, 24)

	send(m,
		thinkDeltaMsg("weighing options"),
		textDeltaMsg("The answer is 42."),
	)

	if len(m.entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(m.entries), m.entries)
	}
	if m.entries[0].kind != entryThinking || m.entries[1].kind != entryAssistant {
		t.Errorf("kinds = %v, %v", m.entries[0].kind, m.entries[1].kind)
	}
}

func TestToolEntryCompletesInPlace(t *testing.T) {
	m := newTestModel(t, 80, 24)
	call := llm.ToolUseBlock{ID: "t1", Name: "read", Input: json.RawMessage(`{"path":"/etc/hosts"}`)}

	send(m, toolCallMsg{Call: call})
	if len(m.entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.entries))
	}
	if m.entries[0].done {
		t.Error("entry should start pending")
	}
	if got := m.rendered[0]; !strings.Contains(got, "…") {
		t.Errorf("pending tool should show an ellipsis, got %q", got)
	}

	send(m, toolResultMsg{Call: call, Result: strings.Repeat("x", 412)})
	if len(m.entries) != 1 {
		t.Fatalf("completing a tool must not add an entry, got %d", len(m.entries))
	}
	entry := m.entries[0]
	if !entry.done || entry.failed || entry.resultLen != 412 {
		t.Errorf("entry = %+v", entry)
	}
	if got := m.rendered[0]; !strings.Contains(got, "412B") {
		t.Errorf("completed tool should show result size, got %q", got)
	}
}

func TestFailedToolRenders(t *testing.T) {
	m := newTestModel(t, 80, 24)
	call := llm.ToolUseBlock{ID: "t1", Name: "boom"}
	send(m,
		toolCallMsg{Call: call},
		toolResultMsg{Call: call, Result: "Error: nope", IsError: true},
	)
	if got := m.rendered[0]; !strings.Contains(got, "failed") {
		t.Errorf("want a failure marker, got %q", got)
	}
}

func TestContinuationLinesAlign(t *testing.T) {
	m := newTestModel(t, 40, 24)
	send(m, textDeltaMsg(strings.Repeat("word ", 40)))

	lines := strings.Split(m.rendered[0], "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping at width 40, got %d line(s)", len(lines))
	}

	for i, line := range lines[1:] {
		if got := leadingSpaces(stripANSI(line)); got != gutterWidth {
			t.Errorf("continuation line %d starts at column %d, want %d: %q",
				i+1, got, gutterWidth, stripANSI(line))
		}
	}
}

func TestMarkedEntryHangsUnderItsText(t *testing.T) {
	m := newTestModel(t, 40, 24)
	send(m, thinkDeltaMsg(strings.Repeat("thought ", 20)))

	lines := strings.Split(m.rendered[0], "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s)", len(lines))
	}
	if !strings.Contains(stripANSI(lines[0]), "⋯") {
		t.Fatalf("no marker on the first line: %q", stripANSI(lines[0]))
	}
	if got, want := leadingSpaces(stripANSI(lines[1])), gutterWidth; got != want {
		t.Errorf("continuation starts at column %d, want %d (under the text, not the marker)", got, want)
	}
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

func TestUsageAccumulates(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m,
		turnMsg{Model: "claude-opus-5", Usage: llm.Usage{InputTokens: 1200, OutputTokens: 400}},
		turnMsg{Model: "claude-opus-5", Usage: llm.Usage{InputTokens: 800, OutputTokens: 150, CacheReadTokens: 900}},
	)
	if m.sessionUsage.InputTokens != 2000 || m.sessionUsage.OutputTokens != 550 || m.sessionUsage.CacheReadTokens != 900 {
		t.Errorf("session usage = %+v", m.sessionUsage)
	}
	if got := m.usageLine(); !strings.Contains(got, "2.0k↑") || !strings.Contains(got, "550↓") {
		t.Errorf("usage line = %q", got)
	}
}

func TestResizeRewraps(t *testing.T) {
	m := newTestModel(t, 100, 24)
	send(m, textDeltaMsg(strings.Repeat("word ", 30)))
	wide := len(strings.Split(m.rendered[0], "\n"))

	m.resize(40, 24)
	narrow := len(strings.Split(m.rendered[0], "\n"))

	if narrow <= wide {
		t.Errorf("narrowing should produce more lines: %d wide vs %d narrow", wide, narrow)
	}
}

func TestViewIncludesChrome(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m, textDeltaMsg("hi"))

	view := m.View()
	for _, want := range []string{"zaino", "anthropic", "claude-opus-5", "xhigh", "⏎ send"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	if got := strings.Count(view, "\n") + 1; got > 24 {
		t.Errorf("view is %d lines, want <= 24", got)
	}
}

func TestResetClears(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m, textDeltaMsg("hi"), turnMsg{Usage: llm.Usage{InputTokens: 10}})
	m.messages = []llm.Message{llm.UserText("hi")}

	m.reset()

	if len(m.messages) != 0 {
		t.Errorf("history not cleared: %+v", m.messages)
	}
	if m.sessionUsage.InputTokens != 0 {
		t.Errorf("usage not reset: %+v", m.sessionUsage)
	}
	if len(m.entries) != 1 || m.entries[0].kind != entryNotice {
		t.Errorf("expected a single notice entry, got %+v", m.entries)
	}
}

func TestEnterQueuesAPromptWhileStreaming(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.input.SetValue("second question")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.input.Value() != "" {
		t.Errorf("the prompt was not taken: %q", m.input.Value())
	}
	if len(m.entries) != 1 || m.entries[0].kind != entryUser {
		t.Fatalf("want the prompt on the transcript, got %+v", m.entries)
	}
	if m.steered != 1 {
		t.Errorf("steered = %d, want 1 waiting", m.steered)
	}
	if got := m.agent.Steered(); len(got) != 1 || got[0].Text() != "second question" {
		t.Errorf("the agent was not handed the prompt: %+v", got)
	}
	if len(m.messages) != 0 {
		t.Errorf("the conversation must not grow until the boundary carries it: %+v", m.messages)
	}
}

func TestWhatWasLeftUnsaidBecomesTheNextPrompt(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.input.SetValue("and then this")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	history := []llm.Message{llm.UserText("first"), {Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "done"}}}}
	m.Update(doneMsg{Messages: history})

	if !m.streaming {
		t.Fatal("a turn that ended well should start again with what was queued")
	}
	if n := len(m.messages); n != 3 || m.messages[2].Text() != "and then this" {
		t.Errorf("messages = %d, want the queued prompt appended: %+v", n, m.messages)
	}
	if m.steered != 0 {
		t.Errorf("steered = %d after it went in", m.steered)
	}
}

func TestWhatWasLeftUnsaidComesBackAfterAnError(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.input.SetValue("and then this")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m.Update(doneMsg{Messages: []llm.Message{llm.UserText("first")}, Err: context.Canceled})

	if m.streaming {
		t.Fatal("an interrupted turn must not start again on its own")
	}
	if m.input.Value() != "and then this" {
		t.Errorf("composer = %q, want the unsent prompt back", m.input.Value())
	}
}

func TestTranscriptNeverTouchesTheRule(t *testing.T) {
	m := newTestModel(t, 80, 24)
	for i := range 40 {
		m.push(entry{kind: entryAssistant, text: fmt.Sprintf("answer %d", i)})
	}

	lines := strings.Split(stripANSI(m.View()), "\n")
	if got := len(lines); got != 24 {
		t.Fatalf("view is %d lines, want 24", got)
	}
	ruleAt := -1
	for i, line := range lines {
		if strings.Contains(line, "───") {
			ruleAt = i
		}
	}
	if ruleAt < 1 {
		t.Fatal("no rule on screen")
	}
	if strings.TrimSpace(lines[ruleAt-1]) != "" {
		t.Errorf("the line above the rule is not blank: %q", lines[ruleAt-1])
	}
}

func TestToolCallsOpenUnderTheBarMidTurn(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.push(entry{kind: entryUser, text: "read it"})
	m.push(entry{kind: entryTool, toolName: "read", toolID: "t1", toolInput: `{"path":"main.go"}`})

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after ⌃k, want the tool call", m.cursor)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.entries[1].expanded {
		t.Error("⏎ on a tool call should open it, even while the turn runs")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.entries[1].expanded {
		t.Error("a second ⏎ should close it again")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.cursor >= 0 {
		t.Errorf("walking off the end should clear the bar, cursor = %d", m.cursor)
	}
}

func TestARunningCommandShowsItsTail(t *testing.T) {
	m := newTestModel(t, 80, 24)
	call := llm.ToolUseBlock{ID: "b1", Name: "bash", Input: []byte(`{"command":"go test ./..."}`)}
	m.Update(toolCallMsg{Call: call})

	m.Update(toolProgressMsg{Call: call, Chunk: "ok  pkg/a\n"})
	m.Update(toolProgressMsg{Call: call, Chunk: "ok  pkg/b\nok  pkg/c\nok  pkg/d\n"})

	view := stripANSI(m.View())
	if strings.Contains(view, "pkg/a") {
		t.Error("the tail should hold only the last few lines")
	}
	for _, want := range []string{"pkg/b", "pkg/c", "pkg/d"} {
		if !strings.Contains(view, want) {
			t.Errorf("the card is missing %q:\n%s", want, view)
		}
	}
	if m.entries[0].done {
		t.Error("progress must not mark the call done")
	}

	m.Update(toolResultMsg{Call: call, Result: "ok  pkg/a\nok  pkg/b\nok  pkg/c\nok  pkg/d\nPASS"})
	view = stripANSI(m.View())
	if strings.Contains(view, "pkg/d") && !m.entries[0].expanded {
		t.Error("once done, the tail should fold away into the card")
	}
	if m.entries[0].toolResult != "ok  pkg/a\nok  pkg/b\nok  pkg/c\nok  pkg/d\nPASS" {
		t.Errorf("the result should replace the partial output, got %q", m.entries[0].toolResult)
	}
}
