package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

const SummaryPrefix = "Summary of the conversation so far:\n\n"

const (
	DefaultWindow     = 200_000
	DefaultReserve    = 16_384
	DefaultKeepRecent = 20_000
)

// Compaction folds the older part of a conversation into one summary so a long
// session keeps fitting. Nil turns it off.
type Compaction struct {
	Window     int
	Reserve    int
	KeepRecent int
}

func (c *Compaction) window() int     { return orDefault(c.Window, DefaultWindow) }
func (c *Compaction) reserve() int    { return orDefault(c.Reserve, DefaultReserve) }
func (c *Compaction) keepRecent() int { return orDefault(c.KeepRecent, DefaultKeepRecent) }

func (c *Compaction) budget() int { return max(c.window()-c.reserve(), c.keepRecent()) }

// The last turn's own numbers are the only true measure of what the provider
// counted; the estimate is what stands in before there has been one.
func (a *Agent) contextSize(history []llm.Message) int {
	if a.used > 0 {
		return a.used
	}
	return estimateTokens(history)
}

func (a *Agent) shouldCompact(history []llm.Message) bool {
	return a.Compaction != nil && len(history) > 1 && a.contextSize(history) > a.Compaction.budget()
}

// Fold compacts on demand, whatever the context is currently costing.
func (a *Agent) Fold(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	if a.Compaction == nil {
		a.Compaction = &Compaction{}
	}
	return a.compact(ctx, history)
}

func (a *Agent) compact(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	cut := cutPoint(history, a.Compaction.keepRecent())
	if cut <= 0 {
		return history, nil
	}

	summary, err := a.summarise(ctx, history[:cut])
	if err != nil {
		return history, err
	}

	kept := append([]llm.Message{llm.UserText(SummaryPrefix + summary)}, history[cut:]...)
	a.used = estimateTokens(kept)

	if a.Hooks.OnCompact != nil {
		a.Hooks.OnCompact(summary, kept)
	}
	return kept, nil
}

// Walks back from the newest message until the recent window is full, then
// moves forward off any message that opens with a tool result: its call would
// be on the other side of the cut, and a result without its call is rejected.
func cutPoint(history []llm.Message, keepRecent int) int {
	cut, size := len(history), 0
	for cut > 0 {
		size += estimateTokens(history[cut-1 : cut])
		cut--
		if size >= keepRecent {
			break
		}
	}

	for cut < len(history) && opensWithToolResult(history[cut]) {
		cut++
	}
	if cut >= len(history) {
		return 0
	}
	return cut
}

func opensWithToolResult(m llm.Message) bool {
	for _, block := range m.Content {
		if _, ok := block.(llm.ToolResultBlock); ok {
			return true
		}
	}
	return false
}

const summarySystem = `You are summarising a coding session so it can continue in a smaller context.
Write for the assistant that will pick it up with no other memory of what happened.

Cover, in prose:
- what the user asked for, in their own terms, including anything they corrected
- what has been done so far, naming the files and symbols involved
- decisions taken and the reasons, especially ones that would otherwise be redone
- what was tried and did not work, so it is not tried again
- what is left to do, and where the work stopped

Be specific. Names, paths and error text are worth more than adjectives.
Do not pad, do not address the user, and do not describe this instruction.`

func (a *Agent) summarise(ctx context.Context, older []llm.Message) (string, error) {
	scribe := &Agent{
		Provider:  a.Provider,
		Model:     a.Model,
		MaxTokens: min(orDefault(a.MaxTokens, DefaultMaxTokens), 8192),
		System:    summarySystem,
	}

	resp, err := scribe.turn(ctx, []llm.Message{llm.UserText(serialise(older))})
	if err != nil {
		return "", fmt.Errorf("compaction: %w", err)
	}

	summary := strings.TrimSpace(resp.Text())
	if summary == "" {
		return "", fmt.Errorf("compaction: the model summarised nothing")
	}
	return summary, nil
}

// Tool traffic is flattened rather than replayed: what matters to a summary is
// that a file was read, not the whole of it.
func serialise(messages []llm.Message) string {
	var b strings.Builder
	b.WriteString("Summarise this conversation.\n\n")

	for _, m := range messages {
		for _, block := range m.Content {
			switch t := block.(type) {
			case llm.TextBlock:
				if strings.TrimSpace(t.Text) == "" {
					continue
				}
				fmt.Fprintf(&b, "%s: %s\n\n", m.Role, t.Text)
			case llm.ToolUseBlock:
				fmt.Fprintf(&b, "called %s %s\n", t.Name, clipRunes(string(t.Input), 300))
			case llm.ToolResultBlock:
				status := "ok"
				if t.IsError {
					status = "failed"
				}
				fmt.Fprintf(&b, "  → %s: %s\n", status, clipRunes(t.Content, 300))
			}
		}
	}
	return b.String()
}

// Four characters to the token is the usual rule of thumb, and it only has to
// be close enough to decide when to ask the provider for the real number.
func estimateTokens(messages []llm.Message) int {
	chars := 0
	for _, m := range messages {
		for _, block := range m.Content {
			switch t := block.(type) {
			case llm.TextBlock:
				chars += len(t.Text)
			case llm.ThinkingBlock:
				chars += len(t.Thinking)
			case llm.ToolUseBlock:
				chars += len(t.Input) + len(t.Name)
			case llm.ToolResultBlock:
				chars += len(t.Content)
			}
		}
		chars += 8
	}
	return chars / 4
}

func clipRunes(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
