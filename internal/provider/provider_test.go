package provider

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func noCredentials(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"GEMINI_API_KEY", "GOOGLE_API_KEY",
		"OPENAI_API_KEY", "XAI_API_KEY", "GROK_API_KEY",
		"OPENROUTER_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

func allCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("GEMINI_API_KEY", "goog-test")
	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	t.Setenv("XAI_API_KEY", "xai-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")
}

func TestNewByName(t *testing.T) {
	noCredentials(t)
	allCredentials(t)

	cases := map[string]string{
		"anthropic":  "anthropic",
		"claude":     "anthropic",
		"gemini":     "gemini",
		"google":     "gemini",
		"ANTHROPIC":  "anthropic",
		"  Gemini ":  "gemini",
		"openai":     "openai",
		"gpt":        "openai",
		"grok":       "grok",
		"xai":        "grok",
		"openrouter": "openrouter",
		"router":     "openrouter",
	}
	for name, want := range cases {
		p, err := New(name)
		if err != nil {
			t.Errorf("New(%q): %v", name, err)
			continue
		}
		if got := p.Name(); got != want {
			t.Errorf("New(%q).Name() = %q, want %q", name, got, want)
		}
	}
}

func TestAutoPrefersAnthropic(t *testing.T) {
	noCredentials(t)
	allCredentials(t)

	for _, name := range []string{"", "auto"} {
		p, err := New(name)
		if err != nil {
			t.Fatalf("New(%q): %v", name, err)
		}
		if got := p.Name(); got != "anthropic" {
			t.Errorf("New(%q) picked %q, want anthropic", name, got)
		}
	}
}

func TestAutoFallsThroughToTheNextCredential(t *testing.T) {
	noCredentials(t)
	t.Setenv("GEMINI_API_KEY", "goog-test")

	p, err := New("auto")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Name(); got != "gemini" {
		t.Errorf("got %q, want gemini", got)
	}
}

func TestAutoWithoutAnyCredentialsSaysWhatIsMissing(t *testing.T) {
	noCredentials(t)

	_, err := New("auto")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
	for _, want := range []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "XAI_API_KEY", "OPENROUTER_API_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}

// Naming a provider with no key is a different failure from having no keys at
// all, and must not be reported as one.
func TestANamedProviderWithoutItsKeyFailsOnItsOwnTerms(t *testing.T) {
	noCredentials(t)

	_, err := New("anthropic")
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Errorf("got the auto-mode error, want the anthropic one: %v", err)
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("error does not name the provider: %v", err)
	}
}

func TestUnknownProviderListsTheKnownOnes(t *testing.T) {
	_, err := New("llama")
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), `"llama"`) {
		t.Errorf("error does not quote the name: %v", err)
	}
	for _, want := range Available() {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not offer %q: %v", want, err)
		}
	}
}

func TestAvailableIsSortedAndStable(t *testing.T) {
	got := Available()
	if !slices.IsSorted(got) {
		t.Errorf("Available() = %v, want sorted", got)
	}
	got[0] = "mutated"
	if Available()[0] == "mutated" {
		t.Error("Available() handed out its own backing array")
	}
}

func TestEveryAvailableNameConstructs(t *testing.T) {
	noCredentials(t)
	allCredentials(t)

	for _, name := range Available() {
		p, err := New(name)
		if err != nil {
			t.Errorf("New(%q): %v", name, err)
			continue
		}
		if p.DefaultModel() == "" {
			t.Errorf("%s has no default model", name)
		}
	}
}

func TestWithHTTPClientReachesBothProviders(t *testing.T) {
	noCredentials(t)
	allCredentials(t)

	for _, name := range Available() {
		if _, err := New(name, WithHTTPClient(nil)); err != nil {
			t.Errorf("New(%q) with an http client: %v", name, err)
		}
	}
}

func TestHasCredentials(t *testing.T) {
	noCredentials(t)

	for _, name := range Available() {
		if HasCredentials(name) {
			t.Errorf("HasCredentials(%q) = true with nothing set", name)
		}
	}

	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	if !HasCredentials("openai") {
		t.Error("HasCredentials(openai) = false with a key set")
	}
	if HasCredentials("grok") {
		t.Error("one provider's key must not vouch for another")
	}
	if HasCredentials("llama") {
		t.Error("HasCredentials of an unknown provider = true")
	}
}

// Every name the frontends offer has to name its own variables when it fails,
// or the credentials hint is a dead end.
func TestEveryProviderExplainsItsOwnCredentials(t *testing.T) {
	noCredentials(t)

	for _, name := range Available() {
		_, err := New(name)
		if err == nil {
			t.Errorf("New(%q) succeeded with no credentials", name)
			continue
		}
		if !strings.Contains(err.Error(), name) {
			t.Errorf("New(%q) error does not name the provider: %v", name, err)
		}
		if !strings.Contains(err.Error(), "_API_KEY") {
			t.Errorf("New(%q) error does not name a variable: %v", name, err)
		}
	}
}
