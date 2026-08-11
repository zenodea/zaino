package sessiontest

import (
	"errors"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func Run(t *testing.T, open func(t *testing.T) session.Repo) {
	t.Helper()

	t.Run("EmptyRepo", func(t *testing.T) { testEmptyRepo(t, open(t)) })
	t.Run("AppendFillsBookkeeping", func(t *testing.T) { testAppend(t, open(t)) })
	t.Run("EntriesSurviveReopening", func(t *testing.T) { testReopen(t, open(t)) })
	t.Run("OpenByPrefix", func(t *testing.T) { testPrefix(t, open(t)) })
	t.Run("ListAndLatest", func(t *testing.T) { testList(t, open(t)) })
}

func testEmptyRepo(t *testing.T, repo session.Repo) {
	if _, ok, err := repo.Latest(); err != nil || ok {
		t.Errorf("Latest on an empty repo = %v, %v; want false, nil", ok, err)
	}
	list, err := repo.List()
	if err != nil || len(list) != 0 {
		t.Errorf("List on an empty repo = %d entries, %v", len(list), err)
	}
	if _, err := repo.Open("nope"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Open of a missing session = %v, want ErrNotFound", err)
	}
}

func testAppend(t *testing.T, repo session.Repo) {
	store, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer store.Close()

	if leaf, err := store.Leaf(); err != nil || leaf != "" {
		t.Errorf("Leaf of a new session = %q, %v; want empty", leaf, err)
	}

	first, err := store.Append(session.Model("anthropic", "claude-opus-5"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	switch {
	case first.ID == "":
		t.Error("stored entry has no ID")
	case first.Parent != "":
		t.Errorf("first entry has parent %q, want none", first.Parent)
	case first.Seq != 1:
		t.Errorf("first entry seq = %d, want 1", first.Seq)
	case first.At.IsZero():
		t.Error("stored entry has no timestamp")
	}

	second, err := store.Append(session.Message(llm.UserText("hello"), nil))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if second.Parent != first.ID {
		t.Errorf("second entry parent = %q, want %q", second.Parent, first.ID)
	}
	if second.Seq != 2 {
		t.Errorf("second entry seq = %d, want 2", second.Seq)
	}

	leaf, err := store.Leaf()
	if err != nil || leaf != second.ID {
		t.Errorf("Leaf = %q, %v; want %q", leaf, err, second.ID)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 2 || entries[0].ID != first.ID || entries[1].ID != second.ID {
		t.Errorf("Entries are not oldest-first: %+v", entries)
	}
}

func testReopen(t *testing.T, repo session.Repo) {
	store, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := store.Meta().ID

	usage := llm.Usage{InputTokens: 12, OutputTokens: 34}
	want := []session.New{
		session.Model("anthropic", "claude-opus-5"),
		session.System("be brief"),
		session.Message(llm.UserText("hello"), nil),
		session.Message(llm.Message{
			Role: llm.RoleAssistant,
			Content: llm.Content{
				llm.ThinkingBlock{Thinking: "hmm", Signature: "sig"},
				llm.TextBlock{Text: "hi"},
			},
		}, &usage),
		session.Thinking(true),
		session.Effort(llm.EffortHigh),
		session.Clear(),
	}
	for _, n := range want {
		if _, err := store.Append(n); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reopened.Close()

	entries, err := reopened.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}

	assistant := entries[3]
	if assistant.Usage == nil || assistant.Usage.InputTokens != 12 {
		t.Errorf("usage did not survive: %+v", assistant.Usage)
	}
	if assistant.Message == nil || len(assistant.Message.Content) != 2 {
		t.Fatalf("assistant content did not survive: %+v", assistant.Message)
	}
	think, ok := assistant.Message.Content[0].(llm.ThinkingBlock)
	if !ok || think.Signature != "sig" {
		t.Errorf("thinking block did not survive: %#v", assistant.Message.Content[0])
	}
	if entries[4].On == nil || !*entries[4].On {
		t.Errorf("thinking entry = %+v, want on", entries[4])
	}

	next, err := reopened.Append(session.Message(llm.UserText("again"), nil))
	if err != nil {
		t.Fatalf("Append after reopen: %v", err)
	}
	if next.Seq != len(want)+1 {
		t.Errorf("seq after reopen = %d, want %d", next.Seq, len(want)+1)
	}
	if next.Parent != entries[len(entries)-1].ID {
		t.Errorf("parent after reopen = %q, want the old leaf %q",
			next.Parent, entries[len(entries)-1].ID)
	}
}

func testPrefix(t *testing.T, repo session.Repo) {
	store, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := store.Meta().ID
	store.Close()

	found, err := repo.Open(id[:8])
	if err != nil {
		t.Fatalf("Open by prefix: %v", err)
	}
	defer found.Close()
	if found.Meta().ID != id {
		t.Errorf("opened %q, want %q", found.Meta().ID, id)
	}
}

func testList(t *testing.T, repo session.Repo) {
	first, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	usage := llm.Usage{InputTokens: 10, OutputTokens: 5}
	first.Append(session.Message(llm.UserText("what is the plan?\nsecond line"), nil))
	first.Append(session.Message(llm.Message{
		Role:    llm.RoleAssistant,
		Content: llm.Content{llm.TextBlock{Text: "this"}},
	}, &usage))
	first.Close()

	second, err := repo.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	secondID := second.Meta().ID
	second.Append(session.Message(llm.UserText("later"), nil))
	second.Close()

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List = %d sessions, want 2", len(list))
	}
	if list[0].ID != secondID {
		t.Errorf("List is not newest-first: got %q first", list[0].ID)
	}

	var older session.Summary
	for _, s := range list {
		if s.ID != secondID {
			older = s
		}
	}
	if older.Messages != 2 {
		t.Errorf("messages = %d, want 2", older.Messages)
	}
	if older.Preview != "what is the plan?" {
		t.Errorf("preview = %q, want the first line of the first prompt", older.Preview)
	}
	if older.Tokens != 15 {
		t.Errorf("tokens = %d, want 15", older.Tokens)
	}

	latest, ok, err := repo.Latest()
	if err != nil || !ok || latest.ID != secondID {
		t.Errorf("Latest = %q, %v, %v; want %q", latest.ID, ok, err, secondID)
	}
}
