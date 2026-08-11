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
