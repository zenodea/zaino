package session_test

import (
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func build(t *testing.T, news ...session.New) session.Context {
	t.Helper()
	repo, err := session.OpenDir(t.TempDir(), "/work")
	if err != nil {
		t.Fatalf("OpenDir: %v", err)
	}
	store, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer store.Close()

	for _, n := range news {
		if _, err := store.Append(n); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	return session.Build(entries)
}

func TestClearCutsMessagesAndKeepsSettings(t *testing.T) {
	before := llm.Usage{InputTokens: 100, OutputTokens: 50}
	after := llm.Usage{InputTokens: 7, OutputTokens: 3}

	c := build(t,
		session.Model("anthropic", "claude-opus-5"),
		session.System("be brief"),
		session.Effort(llm.EffortHigh),
		session.Message(llm.UserText("old question"), nil),
		session.Message(llm.Message{Role: llm.RoleAssistant,
			Content: llm.Content{llm.TextBlock{Text: "old answer"}}}, &before),
		session.Clear(),
		session.Message(llm.UserText("new question"), &after),
	)

	if len(c.Messages) != 1 {
		t.Fatalf("got %d messages, want only the one after the clear: %+v", len(c.Messages), c.Messages)
	}
	if got := c.Messages[0].Text(); got != "new question" {
		t.Errorf("message = %q, want %q", got, "new question")
	}

	if c.Model != "claude-opus-5" || c.Provider != "anthropic" {
		t.Errorf("provider/model = %q/%q, want anthropic/claude-opus-5", c.Provider, c.Model)
	}
	if c.System != "be brief" {
		t.Errorf("system = %q, want it kept across the clear", c.System)
	}
	if c.Effort != llm.EffortHigh {
		t.Errorf("effort = %q, want it kept across the clear", c.Effort)
	}

	if c.Usage.InputTokens != 7 || c.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v, want only what was spent after the clear", c.Usage)
	}
}

func TestLatestSettingWins(t *testing.T) {
	c := build(t,
		session.Model("anthropic", "claude-opus-5"),
		session.Effort(llm.EffortLow),
		session.Model("gemini", "gemini-2.5-flash"),
		session.Effort(""),
		session.Thinking(true),
		session.Thinking(false),
		session.System("first"),
		session.System(""),
	)

	if c.Provider != "gemini" || c.Model != "gemini-2.5-flash" {
		t.Errorf("provider/model = %q/%q, want the last one set", c.Provider, c.Model)
	}
	if c.Effort != "" {
		t.Errorf("effort = %q, want it back to the default", c.Effort)
	}
	if c.Thinking == nil || *c.Thinking {
		t.Errorf("thinking = %v, want off", c.Thinking)
	}
	if c.System != "" {
		t.Errorf("system = %q, want it dropped", c.System)
	}
}

func TestBuildOfNothing(t *testing.T) {
	c := session.Build(nil)
	if len(c.Messages) != 0 || c.Model != "" || c.Thinking != nil {
		t.Errorf("Build(nil) = %+v, want an empty context", c)
	}
}

func TestStripProviderBlocks(t *testing.T) {
	messages := []llm.Message{
		llm.UserText("hello"),
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ThinkingBlock{Thinking: "hmm", Signature: "sig"},
			llm.TextBlock{Text: "hi"},
		}},
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.OpaqueBlock{Type: "server_tool_use", Raw: []byte(`{"type":"server_tool_use"}`)},
		}},
	}

	kept, dropped := session.StripProviderBlocks(messages)
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %d messages, want 2: %+v", len(kept), kept)
	}
	if len(kept[1].Content) != 1 {
		t.Errorf("assistant content = %+v, want just the text", kept[1].Content)
	}
	if kept[1].Text() != "hi" {
		t.Errorf("assistant text = %q, want %q", kept[1].Text(), "hi")
	}
}
