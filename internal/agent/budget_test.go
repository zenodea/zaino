package agent

import (
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/pricing"
)

func TestBudgetAddsUpWhatItCanPrice(t *testing.T) {
	b := NewBudget(pricing.Known(), 1)
	b.Charge("claude-opus-5", llm.Usage{InputTokens: 100_000, OutputTokens: 10_000})
	if got := b.Spent(); got < 0.74 || got > 0.76 {
		t.Errorf("spent = %.3f, want 0.5 in and 0.25 out", got)
	}
	if b.exceeded() {
		t.Error("under the cap, yet exceeded")
	}

	b.Charge("claude-opus-5", llm.Usage{OutputTokens: 10_000})
	if !b.exceeded() {
		t.Errorf("spent %.3f of a $1 cap, not exceeded", b.Spent())
	}
	b.SetCap(0)
	if b.exceeded() {
		t.Error("no cap, yet exceeded")
	}
}

func TestBudgetKeepsWhatItCannotPriceApart(t *testing.T) {
	b := NewBudget(pricing.Known(), 1)
	b.Charge("nobody-knows-this", llm.Usage{InputTokens: 5, OutputTokens: 7})
	if b.Spent() != 0 {
		t.Errorf("spent %.3f on an unpriced model", b.Spent())
	}
	if u := b.Unpriced(); u.InputTokens != 5 || u.OutputTokens != 7 {
		t.Errorf("unpriced = %+v", u)
	}
}
