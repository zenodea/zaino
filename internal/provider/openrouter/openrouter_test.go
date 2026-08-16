package openrouter

import (
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestConfig(t *testing.T) {
	cfg := Config()

	if cfg.Name != "openrouter" {
		t.Errorf("name = %q", cfg.Name)
	}
	if cfg.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKeyEnv[0] != "OPENROUTER_API_KEY" {
		t.Errorf("apiKeyEnv = %v", cfg.APIKeyEnv)
	}
	// Hundreds of models from every vendor: a list compiled here would be
	// both wrong and out of date, so /model asks the API.
	if len(cfg.KnownModels) != 0 {
		t.Errorf("knownModels = %v, want none", cfg.KnownModels)
	}
	if cfg.DefaultModel == "" {
		t.Error("no default model")
	}
}

func TestNewNeedsAKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	if _, err := New(); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestNew(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "openrouter" {
		t.Errorf("name = %q", c.Name())
	}
	if !strings.Contains(c.DefaultModel(), "/") {
		t.Errorf("default model = %q, want a vendor-namespaced id", c.DefaultModel())
	}
	if _, ok := any(c).(llm.ModelFetcher); !ok {
		t.Error("openrouter must be able to fetch its models, having none listed")
	}
}
