package grok

import (
	"slices"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := Config()

	if cfg.Name != "grok" {
		t.Errorf("name = %q", cfg.Name)
	}
	if cfg.BaseURL != "https://api.x.ai/v1" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
	for _, env := range []string{"XAI_API_KEY", "GROK_API_KEY"} {
		if !slices.Contains(cfg.APIKeyEnv, env) {
			t.Errorf("apiKeyEnv = %v, want it to include %s", cfg.APIKeyEnv, env)
		}
	}
	if !slices.Contains(cfg.KnownModels, cfg.DefaultModel) {
		t.Errorf("the default model %q is not among %v", cfg.DefaultModel, cfg.KnownModels)
	}
	// xAI has not moved to max_completion_tokens.
	if !cfg.LegacyMaxTokens {
		t.Error("xai wants max_tokens")
	}
	if cfg.DeveloperRole {
		t.Error("xai wants the system role")
	}
}

func TestNewNeedsAKey(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	if _, err := New(); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestNew(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-test")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "grok" {
		t.Errorf("name = %q", c.Name())
	}
	if !strings.HasPrefix(c.DefaultModel(), "grok-") {
		t.Errorf("default model = %q", c.DefaultModel())
	}
}
