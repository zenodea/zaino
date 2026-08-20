package pricing

import (
	"math"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func TestKnownPricesAndPrefixes(t *testing.T) {
	table := Known()

	cost, ok := table.Cost("claude-opus-5", llm.Usage{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 1_000_000})
	if !ok || math.Abs(cost-(5+2.5+0.5)) > 1e-9 {
		t.Errorf("opus cost = %v %v, want 8.00", cost, ok)
	}
	if _, ok := table.Lookup("claude-sonnet-4-6-20260301"); !ok {
		t.Error("a dated snapshot should find its family")
	}
	if _, ok := table.Lookup("openrouter/anthropic/claude-opus-5"); ok {
		t.Error("two vendor prefixes deep is not a claude id")
	}
	if _, ok := table.Lookup("anthropic/claude-opus-5"); !ok {
		t.Error("a vendor prefix should be looked past")
	}
	if _, ok := table.Lookup("nvidia/nemotron-3-ultra"); ok {
		t.Error("an unknown model must not be priced")
	}
}

func TestTheLastWordWins(t *testing.T) {
	table := Known()
	table.Merge(map[string]llm.Price{"nvidia/nemotron-3-ultra": {Input: 0, Output: 0}})
	if _, ok := table.Lookup("nvidia/nemotron-3-ultra"); !ok {
		t.Error("a merged model should be priced, even at zero")
	}
	table.Set("claude-opus-5", llm.Price{Input: 1, Output: 1})
	if p, _ := table.Lookup("claude-opus-5"); p.Input != 1 {
		t.Errorf("override lost: %+v", p)
	}
}
