package session

import "github.com/zenodea/zaino/internal/llm"

type Context struct {
	Messages []llm.Message

	Provider string
	Model    string
	System   string
	Effort   string
	Thinking *bool

	Usage llm.Usage
}

// Settings come from every entry — a clear should not lose your model —
// while messages come from the last clear onwards.
func Build(entries []Entry) Context {
	var c Context

	start := 0
	for i, e := range entries {
		if e.Type == KindClear {
			start = i + 1
		}
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
