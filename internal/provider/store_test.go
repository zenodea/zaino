package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/credentials"
)

func emptyStore(t *testing.T) *credentials.Store {
	t.Helper()
	return credentials.At(filepath.Join(t.TempDir(), credentials.FileName))
}

func TestAStoredKeyIsUsedWhenTheEnvironmentIsEmpty(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)
	if err := store.SetKey("openai", "sk-stored"); err != nil {
		t.Fatal(err)
	}

	p, err := New("openai", WithCredentials(store))
	if err != nil {
		t.Fatalf("a stored key should be credentials enough: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("name = %q", p.Name())
	}
}

// A key in the shell is the one the user is looking at; the file is only the
// fallback for someone who never exported one.
func TestTheEnvironmentWinsOverTheStore(t *testing.T) {
	noCredentials(t)
	t.Setenv("OPENAI_API_KEY", "sk-from-env")

	store := emptyStore(t)
	if err := store.SetKey("openai", "sk-from-store"); err != nil {
		t.Fatal(err)
	}

	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("authorization")
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	p, err := New("openai", WithCredentials(store))
	if err != nil {
		t.Fatal(err)
	}
	st, err := p.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	if seen != "Bearer sk-from-env" {
		t.Errorf("authorization = %q, want the environment's key", seen)
	}
}

func TestAStoredKeyWorksForEveryKeyProvider(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)

	for _, name := range []string{"anthropic", "gemini", "openai", "grok", "openrouter"} {
		if err := store.SetKey(name, "stored-"+name); err != nil {
			t.Fatal(err)
		}
		if _, err := New(name, WithCredentials(store)); err != nil {
			t.Errorf("New(%q) with a stored key: %v", name, err)
		}
	}
}

func TestAStoreWithNothingInItChangesNothing(t *testing.T) {
	noCredentials(t)

	_, err := New("openai", WithCredentials(emptyStore(t)))
	if err == nil {
		t.Fatal("got nil, want the usual credentials error")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("the error stopped naming the variable: %v", err)
	}
}

// OAuth defers to the ant CLI, which holds and refreshes the tokens.
func TestAStoredOAuthEntryBuildsAnthropic(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)
	if err := store.SetOAuth("anthropic", "work"); err != nil {
		t.Fatal(err)
	}

	p, err := New("anthropic", WithCredentials(store))
	if err != nil {
		t.Fatalf("an oauth entry should be credentials enough: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("name = %q", p.Name())
	}
}

// Only Anthropic has an OAuth path; the others must not silently pretend to.
func TestAnOAuthEntryDoesNotVouchForAKeyOnlyProvider(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)
	if err := store.SetOAuth("openai", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := New("openai", WithCredentials(store)); err == nil {
		t.Fatal("got nil, want an error — openai has no oauth")
	}
}

func TestSetStoreAppliesToPlainNew(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)
	if err := store.SetKey("grok", "xai-stored"); err != nil {
		t.Fatal(err)
	}

	SetStore(store)
	t.Cleanup(func() { SetStore(nil) })

	if _, err := New("grok"); err != nil {
		t.Fatalf("the default store was not consulted: %v", err)
	}
	if !HasCredentials("grok") {
		t.Error("HasCredentials ignored the default store")
	}
}

func TestAutoFindsAStoredCredential(t *testing.T) {
	noCredentials(t)
	store := emptyStore(t)
	if err := store.SetKey("gemini", "goog-stored"); err != nil {
		t.Fatal(err)
	}

	p, err := New("auto", WithCredentials(store))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "gemini" {
		t.Errorf("got %q, want gemini", p.Name())
	}
}
