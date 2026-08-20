// Package pricing puts a dollar figure on usage. The table is small and
// deliberately so: Anthropic's own list prices, whatever a host publishes
// about its models, and whatever the config says — in that order of
// authority, last word wins.
package pricing

import (
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
)

// Anthropic list prices as of 2026-06, per million tokens. Cache reads are a
// tenth of input and cache writes a quarter more, across the line-up.
var anthropic = map[string]llm.Price{
	"claude-fable-5":    {Input: 10, Output: 50},
	"claude-mythos-5":   {Input: 10, Output: 50},
	"claude-opus-5":     {Input: 5, Output: 25},
	"claude-opus-4-8":   {Input: 5, Output: 25},
	"claude-opus-4-7":   {Input: 5, Output: 25},
	"claude-opus-4-6":   {Input: 5, Output: 25},
	"claude-sonnet-5":   {Input: 3, Output: 15},
	"claude-sonnet-4-6": {Input: 3, Output: 15},
	"claude-haiku-4-5":  {Input: 1, Output: 5},
}

type Table struct {
	mu    sync.Mutex
	known map[string]llm.Price
}

func Known() *Table {
	t := &Table{known: make(map[string]llm.Price, len(anthropic))}
	for id, p := range anthropic {
		p.CacheRead, p.CacheWrite = p.Input/10, p.Input*1.25
		t.known[id] = p
	}
	return t
}

func (t *Table) Set(model string, p llm.Price) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.known[strings.ToLower(model)] = p
}

func (t *Table) Merge(prices map[string]llm.Price) {
	for id, p := range prices {
		t.Set(id, p)
	}
}

// Lookup takes the exact id first and the longest known prefix otherwise, so
// a dated snapshot or a vendor prefix still finds its family.
func (t *Table) Lookup(model string) (llm.Price, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	model = strings.ToLower(model)
	if p, ok := t.known[model]; ok {
		return p, true
	}
	if _, after, ok := strings.Cut(model, "/"); ok {
		model = after
	}
	best, found := "", llm.Price{}
	for id, p := range t.known {
		if strings.HasPrefix(model, id) && len(id) > len(best) {
			best, found = id, p
		}
	}
	return found, best != ""
}

func (t *Table) Cost(model string, u llm.Usage) (float64, bool) {
	p, ok := t.Lookup(model)
	if !ok {
		return 0, false
	}
	return p.Cost(u), true
}
