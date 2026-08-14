package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/session/sessiontest"
)

func TestFileRepo(t *testing.T) {
	sessiontest.Run(t, func(t *testing.T) session.Repo {
		t.Helper()
		repo, err := session.OpenDir(t.TempDir(), "/work")
		if err != nil {
			t.Fatalf("OpenDir: %v", err)
		}
		return repo
	})
}

func write(t *testing.T, lines ...string) (*session.FileRepo, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := session.OpenDir(dir, "/work")
	if err != nil {
		t.Fatalf("OpenDir: %v", err)
	}

	id := "20260817-170312-abcd"
	body := strings.Join(append([]string{
		`{"v":1,"id":"` + id + `","started":"2026-08-17T17:03:12Z","cwd":"/work"}`,
	}, lines...), "\n")
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return repo, id
}

func TestTornLastLineIsDroppedAndRepaired(t *testing.T) {
	repo, id := write(t,
		`{"id":"a1","seq":1,"at":"2026-08-17T17:03:13Z","type":"message","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
		`{"id":"b2","seq":2,"at":"2026-08-17T17:03:14Z","type":"mess`,
	)

	store, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open with a torn tail: %v", err)
	}
	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "a1" {
		t.Fatalf("got %d entries, want just the whole one: %+v", len(entries), entries)
	}

	if _, err := store.Append(session.Message(llm.UserText("again"), nil)); err != nil {
		t.Fatalf("Append after repair: %v", err)
	}
	store.Close()

	reopened, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open after repair: %v", err)
	}
	defer reopened.Close()
	entries, _ = reopened.Entries()
	if len(entries) != 2 {
		t.Errorf("after repair and append, got %d entries, want 2: %+v", len(entries), entries)
	}
}

func TestDamageInTheMiddleIsRefused(t *testing.T) {
	repo, id := write(t,
		`{"id":"a1","seq":1,"at":"2026-08-17T17:03:13Z","type":"clear"}`,
		`{"id":"b2","seq":2,"at":"2026-08-17T17:0`,
		`{"id":"c3","seq":3,"at":"2026-08-17T17:03:15Z","type":"clear"}`,
	)

	_, err := repo.Open(id)
	if err == nil {
		t.Fatal("Open of a file damaged in the middle succeeded")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %v, want it to name line 3", err)
	}
}

func TestUnterminatedLastLineStillAppendsCleanly(t *testing.T) {
	repo, id := write(t,
		`{"id":"a1","seq":1,"at":"2026-08-17T17:03:13Z","type":"clear"}`,
	)

	store, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.Append(session.Message(llm.UserText("hello"), nil)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	store.Close()

	reopened, err := repo.Open(id)
	if err != nil {
		t.Fatalf("Open after append: %v", err)
	}
	defer reopened.Close()
	if entries, _ := reopened.Entries(); len(entries) != 2 {
		t.Errorf("got %d entries, want 2: the appended line ran into the old one", len(entries))
	}
}

func TestNewerFormatIsRefused(t *testing.T) {
	dir := t.TempDir()
	repo, err := session.OpenDir(dir, "/work")
	if err != nil {
		t.Fatalf("OpenDir: %v", err)
	}
	id := "20260817-170312-abcd"
	line := `{"v":99,"id":"` + id + `","started":"2026-08-17T17:03:12Z","cwd":"/work"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := repo.Open(id); err == nil {
		t.Fatal("opened a session from a newer format")
	} else if !strings.Contains(err.Error(), "v99") {
		t.Errorf("error = %v, want it to mention the version", err)
	}
}

func TestAmbiguousPrefixIsAnError(t *testing.T) {
	dir := t.TempDir()
	repo, err := session.OpenDir(dir, "/work")
	if err != nil {
		t.Fatalf("OpenDir: %v", err)
	}
	for _, id := range []string{"20260817-170312-aaaa", "20260817-170312-bbbb"} {
		line := `{"v":1,"id":"` + id + `","started":"2026-08-17T17:03:12Z","cwd":"/work"}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	if _, err := repo.Open("20260817"); err == nil {
		t.Fatal("an ambiguous prefix opened something")
	} else if !strings.Contains(err.Error(), "matches") {
		t.Errorf("error = %v, want it to say what it matched", err)
	}
}

func newStore(t *testing.T) session.Store {
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
	return store
}

func TestCompactionIsABoundaryLikeAClear(t *testing.T) {
	store := newStore(t)

	store.Append(session.Message(llm.UserText("the old question"), nil))
	store.Append(session.Message(llm.UserText("more old talk"), nil))
	store.Append(session.Compacted("they asked about the loop"))
	store.Append(session.Message(llm.UserText("the recent one"), nil))

	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	ctx := session.Build(entries)

	if len(ctx.Messages) != 2 {
		t.Fatalf("got %d messages, want the summary and what followed it: %+v", len(ctx.Messages), ctx.Messages)
	}
	if !strings.HasPrefix(ctx.Messages[0].Text(), session.SummaryPrefix) {
		t.Errorf("first message = %q, want the summary rebuilt from the entry", ctx.Messages[0].Text())
	}
	if !strings.Contains(ctx.Messages[0].Text(), "asked about the loop") {
		t.Errorf("summary text lost: %q", ctx.Messages[0].Text())
	}
	if ctx.Messages[1].Text() != "the recent one" {
		t.Errorf("second message = %q", ctx.Messages[1].Text())
	}
	if ctx.Summary == "" {
		t.Error("Context.Summary is empty")
	}
}

// A clear after a compaction means start over, summary included.
func TestClearDropsAnEarlierSummary(t *testing.T) {
	store := newStore(t)

	store.Append(session.Compacted("a summary"))
	store.Append(session.Message(llm.UserText("after the summary"), nil))
	store.Append(session.Clear())
	store.Append(session.Message(llm.UserText("a fresh start"), nil))

	entries, _ := store.Entries()
	ctx := session.Build(entries)

	if len(ctx.Messages) != 1 || ctx.Messages[0].Text() != "a fresh start" {
		t.Errorf("messages = %+v, want only what came after the clear", ctx.Messages)
	}
	if ctx.Summary != "" {
		t.Errorf("Summary = %q, want the clear to have dropped it", ctx.Summary)
	}
}

func TestASecondCompactionSupersedesTheFirst(t *testing.T) {
	store := newStore(t)

	store.Append(session.Compacted("first summary"))
	store.Append(session.Message(llm.UserText("middle"), nil))
	store.Append(session.Compacted("second summary"))
	store.Append(session.Message(llm.UserText("latest"), nil))

	entries, _ := store.Entries()
	ctx := session.Build(entries)

	if len(ctx.Messages) != 2 {
		t.Fatalf("got %d messages, want the newest summary and what followed", len(ctx.Messages))
	}
	if !strings.Contains(ctx.Messages[0].Text(), "second summary") {
		t.Errorf("summary = %q, want the newest one", ctx.Messages[0].Text())
	}
}

// What was kept is written again after the summary, so reading the log back is
// a matter of starting at the boundary and going forwards.
func TestRecorderWritesTheKeptMessagesAfterTheSummary(t *testing.T) {
	store := newStore(t)
	rec := session.NewRecorder(store)

	rec.Messages([]llm.Message{llm.UserText("one"), llm.UserText("two"), llm.UserText("three")})

	kept := []llm.Message{
		llm.UserText(session.SummaryPrefix + "what happened"),
		llm.UserText("three"),
	}
	if err := rec.Compact("what happened", kept); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	rec.Messages(append(kept, llm.UserText("four")))

	entries, _ := store.Entries()
	ctx := session.Build(entries)

	var texts []string
	for _, m := range ctx.Messages {
		texts = append(texts, m.Text())
	}
	want := []string{session.SummaryPrefix + "what happened", "three", "four"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("rebuilt %q, want %q", texts, want)
	}
}
