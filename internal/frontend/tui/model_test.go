package tui

import (
	"encoding/json"
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

func TestHangingIndent(t *testing.T) {
	m := newTestModel(t, 40, 24)
	send(m, textDeltaMsg(strings.Repeat("word ", 40)))

	lines := strings.Split(m.rendered[0], "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping at width 40, got %d line(s)", len(lines))
	}
	if !strings.HasPrefix(lines[0], "◆") {
		t.Errorf("first line should carry the marker, got %q", lines[0])
	}
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, strings.Repeat(" ", gutterWidth)) {
			t.Errorf("continuation line %d not indented into the gutter: %q", i+1, line)
		}
	}
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

func TestEnterIgnoredWhileStreaming(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.streaming = true
	m.input.SetValue("second question")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.input.Value() != "second question" {
		t.Errorf("input should be left intact, got %q", m.input.Value())
	}
	if len(m.entries) != 0 {
		t.Errorf("no entry should be pushed, got %+v", m.entries)
	}
}
