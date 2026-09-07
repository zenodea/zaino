package agent

import (
	"fmt"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/pricing"
)

type Budget struct {
	Prices *pricing.Table

	mu       sync.Mutex
	cap      float64
	spent    float64
	unpriced llm.Usage
}

func NewBudget(prices *pricing.Table, cap float64) *Budget {
	return &Budget{Prices: prices, cap: cap}
}

func (b *Budget) Charge(model string, u llm.Usage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cost, ok := b.Prices.Cost(model, u)
	if !ok {
		b.unpriced.InputTokens += u.InputTokens
		b.unpriced.OutputTokens += u.OutputTokens
		b.unpriced.ThinkingTokens += u.ThinkingTokens
		b.unpriced.CacheReadTokens += u.CacheReadTokens
		b.unpriced.CacheWriteTokens += u.CacheWriteTokens
		return
	}
	b.spent += cost
}

func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

func (b *Budget) Cap() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cap
}

func (b *Budget) SetCap(dollars float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cap = dollars
}

func (b *Budget) Unpriced() llm.Usage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unpriced
}

func (b *Budget) exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cap > 0 && b.spent >= b.cap
}

type SpendLimitError struct {
	Spent, Cap float64
}

func (e *SpendLimitError) Error() string {
	return fmt.Sprintf("agent: spend cap reached: $%.2f of $%.2f", e.Spent, e.Cap)
}
