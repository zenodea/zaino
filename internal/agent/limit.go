package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

// ParseTokens reads a size the way it gets written: 200k, 200000, 1M, off.
func ParseTokens(arg string) (int, error) {
	text := strings.ToLower(strings.TrimSpace(arg))
	switch text {
	case "off", "none", "no", "0":
		return 0, nil
	}

	scale := 1
	switch {
	case strings.HasSuffix(text, "k"):
		scale, text = 1000, strings.TrimSuffix(text, "k")
	case strings.HasSuffix(text, "m"):
		scale, text = 1_000_000, strings.TrimSuffix(text, "m")
	}

	n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("not a token count: %q", arg)
	}
	return int(n * float64(scale)), nil
}

// ContextLimitError stops a turn that would have carried the session past its
// ceiling. Nothing was sent when it is returned.
type ContextLimitError struct {
	Used  int
	Limit int

	// Whether the provider counted this itself, or it had to be estimated here.
	Exact bool
}

func (e *ContextLimitError) Error() string {
	if e.Exact {
		return fmt.Sprintf("agent: context limit reached: %d tokens of %d", e.Used, e.Limit)
	}
	return fmt.Sprintf("agent: context limit reached: about %d tokens of %d", e.Used, e.Limit)
}

// AllowOnce lets the next run through the ceiling. It is spent on that run
// whether or not the run turns out to need it.
func (a *Agent) AllowOnce() { a.allowOnce = true }

// The meter holds the last count known to be exact and how much of the history
// it covered, so a turn only has to price what was added since. It belongs to
// one model: another tokenises the same conversation differently.
type meter struct {
	tokens int
	prefix int
	model  string
	valid  bool
}

func (a *Agent) remeasured(tokens, prefix int) {
	a.meter = meter{tokens: tokens, prefix: prefix, model: a.Model, valid: true}
}

// Compaction rewrites the history the meter was counted against.
func (a *Agent) forget() { a.meter.valid = false }

func (a *Agent) baseline(history []llm.Message) (base int, since []llm.Message, ok bool) {
	m := a.meter
	if !m.valid || m.model != a.Model || m.prefix > len(history) {
		return 0, history, false
	}
	return m.tokens, history[m.prefix:], true
}

// A token is never made of fewer than one byte, so pricing everything added
// since the last exact count at a token per byte gives a ceiling the real
// count cannot pass. While that ceiling stays under the limit there is nothing
// worth asking the provider about.
func (a *Agent) mightExceed(history []llm.Message) bool {
	base, since, ok := a.baseline(history)
	return !ok || base+byteSize(since) > a.MaxContext
}

// Measure returns what the next request would occupy, and whether the provider
// counted it. The exact count is bought only when it could change the answer.
func (a *Agent) measure(ctx context.Context, history []llm.Message) (int, bool) {
	if counter, ok := a.Provider.(llm.TokenCounter); ok && a.mightExceed(history) {
		if n, err := counter.CountTokens(ctx, a.request(history)); err == nil {
			a.remeasured(n, len(history))
			return n, true
		}
	}
	base, since, _ := a.baseline(history)
	return base + estimateTokens(since), false
}

func (a *Agent) withinLimit(ctx context.Context, history []llm.Message) error {
	if a.MaxContext <= 0 {
		return nil
	}
	size, exact := a.measure(ctx, history)
	if size <= a.MaxContext {
		return nil
	}
	return &ContextLimitError{Used: size, Limit: a.MaxContext, Exact: exact}
}

// Everything the provider charges for the whole request, not the fresh part of
// it: cached tokens sit in the window like any other.
func contextTokens(u llm.Usage) int {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens +
		u.OutputTokens + u.ThinkingTokens
}

// Generous where estimateTokens is fair: this one is only ever used as an
// upper bound, so the wire's own punctuation is paid for block by block.
const blockOverhead = 64

func byteSize(messages []llm.Message) int {
	bytes := 0
	for _, m := range messages {
		for _, block := range m.Content {
			switch t := block.(type) {
			case llm.TextBlock:
				bytes += len(t.Text)
			case llm.ThinkingBlock:
				bytes += len(t.Thinking) + len(t.Signature)
			case llm.ToolUseBlock:
				bytes += len(t.Input) + len(t.Name) + len(t.ID) + len(t.Signature)
			case llm.ToolResultBlock:
				bytes += len(t.Content) + len(t.ToolUseID)
			case llm.OpaqueBlock:
				bytes += len(t.Raw)
			}
			bytes += blockOverhead
		}
		bytes += blockOverhead
	}
	return bytes
}
