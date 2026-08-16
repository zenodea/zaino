package openai

import (
	"slices"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := Config()

	if cfg.Name != "openai" {
		t.Errorf("name = %q", cfg.Name)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
	if !slices.Contains(cfg.APIKeyEnv, "OPENAI_API_KEY") {
		t.Errorf("apiKeyEnv = %v", cfg.APIKeyEnv)
	}
	if !slices.Contains(cfg.KnownModels, cfg.DefaultModel) {
		t.Errorf("the default model %q is not among %v", cfg.DefaultModel, cfg.KnownModels)
	}
	if !cfg.ReasoningEffort {
		t.Error("openai understands reasoning_effort")
	}
	// Newer models reject max_tokens outright.
	if cfg.LegacyMaxTokens {
		t.Error("openai wants max_completion_tokens")
	}
}

func TestNewNeedsAKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := New(); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestNew(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "openai" {
		t.Errorf("name = %q", c.Name())
	}
	if !strings.HasPrefix(c.DefaultModel(), "gpt-") {
		t.Errorf("default model = %q", c.DefaultModel())
	}
}
