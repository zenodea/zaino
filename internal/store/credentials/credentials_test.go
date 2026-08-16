package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	return At(filepath.Join(t.TempDir(), FileName))
}

func TestAMissingFileIsNotAnError(t *testing.T) {
	s := tempStore(t)

	entry, ok, err := s.Lookup("anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("found %+v in an empty store", entry)
	}
	if got := s.Providers(); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestSetKeyRoundTrips(t *testing.T) {
	s := tempStore(t)
	if err := s.SetKey("openai", "sk-secret"); err != nil {
		t.Fatal(err)
	}

	entry, ok, err := s.Lookup("openai")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("nothing stored")
	}
	if entry.Method != APIKey || entry.Key != "sk-secret" {
		t.Errorf("got %+v", entry)
	}
}

func TestAKeySurvivesAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := At(path).SetKey("grok", "xai-secret"); err != nil {
		t.Fatal(err)
	}

	entry, ok, err := At(path).Lookup("grok")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || entry.Key != "xai-secret" {
		t.Errorf("got %+v, ok=%v", entry, ok)
	}
}

func TestSetOAuthStoresNoSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := At(path)
	if err := s.SetOAuth("anthropic", "work"); err != nil {
		t.Fatal(err)
	}

	entry, _, err := s.Lookup("anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Method != OAuth {
		t.Errorf("method = %q, want oauth", entry.Method)
	}
	if entry.Profile != "work" {
		t.Errorf("profile = %q", entry.Profile)
	}
	if entry.Key != "" {
		t.Error("an oauth entry must not carry a key — ant holds the tokens")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"key"`) {
		t.Errorf("the file has a key field:\n%s", body)
	}
}

// The file holds API keys in the clear, so nobody else may read it.
func TestTheFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := At(path)
	if err := s.SetKey("openai", "sk-secret"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != Perm {
		t.Errorf("mode = %v, want %v", got, Perm)
	}
}

func TestAnOverwriteStaysOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"providers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := At(path).SetKey("openai", "sk-secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != Perm {
		t.Errorf("mode = %v, want %v — a loose file must be tightened", got, Perm)
	}
}

func TestSetReplacesAnEarlierEntry(t *testing.T) {
	s := tempStore(t)
	if err := s.SetKey("openai", "sk-first"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKey("openai", "sk-second"); err != nil {
		t.Fatal(err)
	}

	entry, _, _ := s.Lookup("openai")
	if entry.Key != "sk-second" {
		t.Errorf("key = %q", entry.Key)
	}
	if got := s.Providers(); len(got) != 1 {
		t.Errorf("got %v, want one provider", got)
	}
}

func TestProvidersIsSorted(t *testing.T) {
	s := tempStore(t)
	for _, name := range []string{"openai", "anthropic", "grok"} {
		if err := s.SetKey(name, "k"); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Providers()
	want := "anthropic,grok,openai"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestRemove(t *testing.T) {
	s := tempStore(t)
	if err := s.SetKey("openai", "sk-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("openai"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Lookup("openai"); ok {
		t.Error("the entry survived removal")
	}

	if err := s.Remove("never-stored"); err != nil {
		t.Errorf("removing what is not there should be quiet: %v", err)
	}
}

func TestSeveralProvidersCoexist(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := At(path)
	if err := s.SetKey("openai", "sk-openai"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOAuth("anthropic", ""); err != nil {
		t.Fatal(err)
	}

	reopened := At(path)
	if entry, ok, _ := reopened.Lookup("openai"); !ok || entry.Key != "sk-openai" {
		t.Errorf("openai = %+v", entry)
	}
	if entry, ok, _ := reopened.Lookup("anthropic"); !ok || entry.Method != OAuth {
		t.Errorf("anthropic = %+v", entry)
	}
}

func TestAnUnreadableFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("{not json"), Perm); err != nil {
		t.Fatal(err)
	}

	_, _, err := At(path).Lookup("openai")
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error does not name the file: %v", err)
	}
}

func TestOpenUsesTheDataDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "zaino", FileName)
	if s.Path() != want {
		t.Errorf("path = %q, want %q", s.Path(), want)
	}
}
