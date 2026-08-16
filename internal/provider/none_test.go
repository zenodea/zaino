package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestNoneCarriesItsReason(t *testing.T) {
	boom := errors.New("no key anywhere")
	p := None(boom)

	if !IsNone(p) {
		t.Error("IsNone = false")
	}
	if got := Reason(p); !errors.Is(got, boom) {
		t.Errorf("Reason = %v, want the original", got)
	}

	_, err := p.Stream(context.Background(), llm.Request{})
	if !errors.Is(err, boom) {
		t.Errorf("Stream = %v, want the original reason", err)
	}
}

func TestNoneWithoutAReasonStillExplainsItself(t *testing.T) {
	p := None(nil)
	if _, err := p.Stream(context.Background(), llm.Request{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

// The frontends render the name and model, so neither may panic or lie.
func TestNoneIsSafeToRender(t *testing.T) {
	p := None(nil)
	if p.Name() != NoneName {
		t.Errorf("name = %q", p.Name())
	}
	if p.DefaultModel() != "" {
		t.Errorf("model = %q, want empty", p.DefaultModel())
	}
	if lister, ok := p.(llm.ModelLister); ok && len(lister.Models()) != 0 {
		t.Error("an unconfigured provider must not list models")
	}
}

func TestNoneNameIsNotARealProvider(t *testing.T) {
	for _, name := range Available() {
		if name == NoneName {
			t.Fatalf("%q collides with a real provider", NoneName)
		}
	}
	if _, err := New(NoneName); err == nil {
		t.Error("the placeholder name should not construct a provider")
	}
}

func TestARealProviderIsNotNone(t *testing.T) {
	noCredentials(t)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	p, err := New("openai")
	if err != nil {
		t.Fatal(err)
	}
	if IsNone(p) {
		t.Error("IsNone = true for a real provider")
	}
	if Reason(p) != nil {
		t.Error("a real provider carries no reason")
	}
}
