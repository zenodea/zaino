package session

import "github.com/zenodea/zaino/internal/llm"

// What a folded-up conversation is introduced with, so the model knows the
// difference between a summary and something you said.
const SummaryPrefix = "Summary of the conversation so far:\n\n"

type Context struct {
	Messages []llm.Message
	Summary  string

	Provider string
	Model    string
	System   string
	Effort   string
	Thinking *bool
	Limit    *int

	Usage llm.Usage
}

// Settings come from every entry — a clear should not lose your model —
// while messages come from the last clear onwards.
func Build(entries []Entry) Context {
	var c Context

	// A summary is a boundary like a clear, except that what came before it
	// is not forgotten so much as folded into one message.
	start, summary := 0, ""
	for i, e := range entries {
		switch e.Type {
		case KindClear:
			start, summary = i+1, ""
		case KindCompact:
			start, summary = i+1, e.Text
		}
	}
	c.Summary = summary
	if summary != "" {
		c.Messages = append(c.Messages, llm.UserText(SummaryPrefix+summary))
	}

	for _, e := range entries {
		switch e.Type {
		case KindModel:
			c.Provider, c.Model = e.Provider, e.Model
		case KindSystem:
			c.System = e.Text
		case KindEffort:
			c.Effort = e.Level
		case KindThinking:
			c.Thinking = e.On
		case KindLimit:
			c.Limit = e.Tokens
		}
	}

	for _, e := range entries[start:] {
		if e.Type != KindMessage || e.Message == nil {
			continue
		}
		c.Messages = append(c.Messages, *e.Message)
		if e.Usage != nil {
			c.Usage.InputTokens += e.Usage.InputTokens
			c.Usage.OutputTokens += e.Usage.OutputTokens
			c.Usage.ThinkingTokens += e.Usage.ThinkingTokens
			c.Usage.CacheReadTokens += e.Usage.CacheReadTokens
			c.Usage.CacheWriteTokens += e.Usage.CacheWriteTokens
		}
	}
	return c
}

func StripProviderBlocks(messages []llm.Message) ([]llm.Message, int) {
	dropped := 0
	out := make([]llm.Message, 0, len(messages))

	for _, m := range messages {
		content := make(llm.Content, 0, len(m.Content))
		for _, block := range m.Content {
			switch b := block.(type) {
			case llm.ThinkingBlock, llm.OpaqueBlock:
				dropped++
			case llm.ToolUseBlock:
				// The call still belongs in the history; only the signature is
				// unreadable over there.
				b.Signature = ""
				content = append(content, b)
			default:
				content = append(content, block)
			}
		}
		if len(content) == 0 {
			continue
		}
		m.Content = content
		out = append(out, m)
	}
	return out, dropped
}
