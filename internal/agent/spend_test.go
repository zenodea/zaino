package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/pricing"
)

func TestARunPastTheSpendCapStopsBeforeSending(t *testing.T) {
	ag, api := newTestAgent(t, textTurn("first"), textTurn("second"))
	prices := pricing.Known()
	prices.Set("test-model", llm.Price{Input: 1_000_000, Output: 1_000_000})
	ag.Budget = NewBudget(prices, 1)

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("hi")}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := ag.Budget.Spent(); got < 10 {
		t.Errorf("spent = %.2f, want the turn's tokens charged", got)
	}

	_, err := ag.Run(context.Background(), []llm.Message{llm.UserText("again")})
	var over *SpendLimitError
	if !errors.As(err, &over) {
		t.Fatalf("second run: %v, want a spend limit error", err)
	}
	if over.Cap != 1 || over.Spent < 10 {
		t.Errorf("error = %+v", over)
	}
	if len(api.requests) != 1 {
		t.Errorf("%d requests went out, want the second held back", len(api.requests))
	}

	ag.Budget.SetCap(0)
	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("again")}); err != nil {
		t.Errorf("with the cap off: %v", err)
	}
}
