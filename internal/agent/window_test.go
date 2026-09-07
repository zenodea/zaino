package agent

import (
	"context"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

type namedProvider struct{ model string }

func (p namedProvider) Name() string         { return "named" }
func (p namedProvider) DefaultModel() string { return p.model }
func (p namedProvider) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return nil, nil
}

func TestWindowForMatchesTheLongestFamily(t *testing.T) {
	cases := map[string]int{
		"claude-opus-5":                 200_000,
		"anthropic/claude-sonnet-5":     200_000,
		"gemini-2.5-pro":                1_000_000,
		"gpt-5-mini":                    400_000,
		"gpt-4.1-nano":                  1_000_000,
		"o3-pro":                        200_000,
		"grok-4-fast":                   256_000,
		"something-nobody-has-heard-of": 0,
	}
	for model, want := range cases {
		if got := WindowFor(model); got != want {
			t.Errorf("WindowFor(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestSetWindowOverridesTheTable(t *testing.T) {
	SetWindow("gpt-5-mini", 123_000)
	defer SetWindow("gpt-5-mini", 400_000)
	if got := WindowFor("gpt-5-mini"); got != 123_000 {
		t.Errorf("got %d, want the override", got)
	}
}

func TestTheWindowFollowsTheModel(t *testing.T) {
	a := &Agent{Provider: namedProvider{model: "gemini-2.5-flash"}, Compaction: &Compaction{}}
	if got := a.Window(); got != 1_000_000 {
		t.Errorf("provider default → %d, want gemini's", got)
	}
	a.Model = "claude-opus-5"
	if got := a.Window(); got != 200_000 {
		t.Errorf("after /model → %d, want claude's", got)
	}
	a.Model = "unknown-model"
	if got := a.Window(); got != DefaultWindow {
		t.Errorf("unknown model → %d, want the default", got)
	}
	a.Compaction.Window = 50_000
	if got := a.Window(); got != 50_000 {
		t.Errorf("explicit window → %d, want it to win", got)
	}
}
