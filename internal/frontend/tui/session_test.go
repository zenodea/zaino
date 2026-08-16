package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/recall"
	"github.com/zenodea/zaino/internal/store/session"
)

func key(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

func TestRecallWalksBackFromAnEmptyBox(t *testing.T) {
	m := newTestModel(t, 80, 24)
	list := recall.New()
	list.Add("first prompt")
	list.Add("second prompt")
	m.UseRecall(list)

	m.Update(key("up"))
	if got := m.input.Value(); got != "second prompt" {
		t.Fatalf("after ↑ the box holds %q, want the newest prompt", got)
	}
	m.Update(key("up"))
	if got := m.input.Value(); got != "first prompt" {
		t.Fatalf("after ↑↑ the box holds %q, want the older prompt", got)
	}

	m.Update(key("down"))
	m.Update(key("down"))
	if got := m.input.Value(); got != "" {
		t.Errorf("walking back to the draft left %q, want the empty box back", got)
	}
}

func TestRecallKeepsTheDraft(t *testing.T) {
	m := newTestModel(t, 80, 24)
	list := recall.New()
	list.Add("old prompt")
	m.UseRecall(list)

	typeLine(m, "half written")
	m.input.CursorStart()

	m.Update(key("up"))
	if got := m.input.Value(); got != "old prompt" {
		t.Fatalf("recalled %q, want the old prompt", got)
	}
	m.Update(key("down"))
	if got := m.input.Value(); got != "half written" {
		t.Errorf("after ↓ the box holds %q, want the draft back", got)
	}
}

func TestRecallLeavesTheCursorAloneMidLine(t *testing.T) {
	m := newTestModel(t, 80, 24)
	list := recall.New()
	list.Add("old prompt")
	m.UseRecall(list)

	typeLine(m, "still writing this")
	m.input.CursorEnd()

	m.Update(key("up"))
	if got := m.input.Value(); got != "still writing this" {
		t.Errorf("↑ mid-line overwrote the box with %q", got)
	}
}

func TestSubmitRemembersThePrompt(t *testing.T) {
	m := newTestModel(t, 80, 24)
	list := recall.New()
	m.UseRecall(list)

	typeLine(m, "how does this work?")
	m.Update(key("enter"))

	if lines := list.Lines(); len(lines) != 1 || lines[0] != "how does this work?" {
		t.Errorf("recall holds %q, want the prompt just sent", lines)
	}
}

func openStore(t *testing.T) (session.Repo, *session.Recorder) {
	t.Helper()
	repo, err := session.OpenDir(t.TempDir(), "/work")
	if err != nil {
		t.Fatalf("OpenDir: %v", err)
	}
	store, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return repo, session.NewRecorder(store)
}

func entryKinds(t *testing.T, repo session.Repo, id string) []session.Kind {
	t.Helper()
	store, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	kinds := make([]session.Kind, len(entries))
	for i, e := range entries {
		kinds[i] = e.Type
	}
	return kinds
}

func TestTurnIsRecordedWithItsUsage(t *testing.T) {
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)

	typeLine(m, "hello")
	m.Update(key("enter"))

	reply := llm.Message{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "hi"}}}
	send(m,
		turnMsg{Model: "claude-opus-5", Usage: llm.Usage{InputTokens: 9, OutputTokens: 4}},
		doneMsg{Messages: []llm.Message{llm.UserText("hello"), reply}},
	)

	store, err := repo.Open(rec.ID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	entries, _ := store.Entries()

	if len(entries) != 2 {
		t.Fatalf("recorded %d entries, want the prompt and the reply: %+v", len(entries), entries)
	}
	if entries[0].Message == nil || entries[0].Message.Text() != "hello" {
		t.Errorf("first entry = %+v, want the prompt", entries[0])
	}
	if entries[0].Usage != nil {
		t.Errorf("the prompt should carry no usage, got %+v", entries[0].Usage)
	}
	if entries[1].Usage == nil || entries[1].Usage.InputTokens != 9 {
		t.Errorf("the reply should carry the turn's usage, got %+v", entries[1].Usage)
	}
}

func TestClearIsRecordedRatherThanDeleted(t *testing.T) {
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)
	id := rec.ID()

	typeLine(m, "hello")
	m.Update(key("enter"))
	send(m, doneMsg{Messages: []llm.Message{llm.UserText("hello")}})

	typeLine(m, "/clear !")
	m.Update(key("enter"))

	typeLine(m, "fresh start")
	m.Update(key("enter"))
	send(m, doneMsg{Messages: []llm.Message{llm.UserText("fresh start")}})

	kinds := entryKinds(t, repo, id)
	want := []session.Kind{session.KindMessage, session.KindClear, session.KindMessage}
	if len(kinds) != len(want) {
		t.Fatalf("entries = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("entries = %v, want %v", kinds, want)
		}
	}

	store, _ := repo.Open(id)
	defer store.Close()
	entries, _ := store.Entries()
	if got := session.Build(entries); len(got.Messages) != 1 {
		t.Errorf("context after the clear has %d messages, want 1", len(got.Messages))
	}
}

func TestRestorePutsTheConversationBackOnScreen(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.Restore(session.Context{
		Messages: []llm.Message{
			llm.UserText("what is 2+2?"),
			{Role: llm.RoleAssistant, Content: llm.Content{
				llm.ThinkingBlock{Thinking: "adding"},
				llm.ToolUseBlock{Name: "calc", Input: []byte(`{"expr":"2+2"}`)},
			}},
			{Role: llm.RoleUser, Content: llm.Content{
				llm.ToolResultBlock{ToolUseID: "t1", Content: "4"},
			}},
			{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "4."}}},
		},
		Usage: llm.Usage{InputTokens: 20, OutputTokens: 5},
	})

	kinds := make([]entryKind, len(m.entries))
	for i, e := range m.entries {
		kinds[i] = e.kind
	}
	want := []entryKind{entryUser, entryTool, entryAssistant}
	if len(kinds) != len(want) {
		t.Fatalf("restored entries = %v, want %v (reasoning is left out)", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("restored entries = %v, want %v", kinds, want)
		}
	}

	if m.entries[1].resultLen != 1 || !m.entries[1].done {
		t.Errorf("the restored tool call should show as finished: %+v", m.entries[1])
	}
	if m.sessionUsage.InputTokens != 20 {
		t.Errorf("usage = %+v, want what the session had spent", m.sessionUsage)
	}
	if len(m.messages) != 4 {
		t.Errorf("restored %d messages to send, want 4", len(m.messages))
	}
}

func TestNothingIsRecordedWithoutAStore(t *testing.T) {
	m := newTestModel(t, 80, 24)

	typeLine(m, "hello")
	m.Update(key("enter"))
	send(m, doneMsg{Messages: []llm.Message{llm.UserText("hello")}})

	typeLine(m, "/clear !")
	m.Update(key("enter"))

	if m.sessionID() != "" {
		t.Errorf("session id = %q, want none", m.sessionID())
	}
	if m.saveFailed {
		t.Error("not saving should not be reported as a failure to save")
	}
}

func TestRestoredToolCallsCanStillBeOpened(t *testing.T) {
	m := newTestModel(t, 90, 24)
	m.ready = true

	call := llm.ToolUseBlock{ID: "t1", Name: "grep",
		Input: json.RawMessage(`{"path":"internal","pattern":"panic"}`)}
	m.Restore(session.Context{Messages: []llm.Message{
		{Role: llm.RoleAssistant, Content: llm.Content{call}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "t1", Content: "agent.go:42:\tpanic(err)\n"}}},
	}})

	e, ok := lastTool(m)
	if !ok {
		t.Fatal("no tool entry was restored")
	}
	if e.toolInput == "" {
		t.Error("restored tool call lost its arguments")
	}
	if e.toolResult == "" {
		t.Error("restored tool call lost its result")
	}

	if line := stripANSI(e.toolLine(70)); strings.Contains(line, `{"`) {
		t.Errorf("restored line shows raw JSON instead of a summary: %q", line)
	}

	e.expanded = true
	if detail := stripANSI(e.detail(70)); strings.Contains(detail, "nothing to show") {
		t.Errorf("restored tool call has nothing to expand:\n%s", detail)
	}
}

func lastTool(m *Model) (entry, bool) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind == entryTool {
			return m.entries[i], true
		}
	}
	return entry{}, false
}
