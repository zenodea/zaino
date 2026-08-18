package session

import (
	"slices"

	"github.com/zenodea/zaino/internal/llm"
)

// What a folded-up conversation is introduced with, so the model knows the
// difference between a summary and something you said.
const SummaryPrefix = "Summary of the conversation so far:\n\n"

type Context struct {
	Messages []llm.Message
	Summary  string
	Tasks    []TaskBody

	// The entry each message came from, one for one with Messages. A summary
	// rebuilt from a compaction has no entry, and the empty mark it leaves is
	// what says it cannot be gone back past.
	Marks []string

	Provider string
	Model    string
	System   string
	Effort   string
	Thinking *bool
	Limit    *int

	Usage llm.Usage
}

// The file holds a tree: a conversation taken up again from an earlier turn
// leaves what it abandoned behind, still on disk. The path is the line from
// the newest entry back to the root, and a file that never branched is all of
// itself.
func Path(entries []Entry) []Entry {
	if len(entries) == 0 {
		return nil
	}
	return PathTo(entries, entries[len(entries)-1].ID)
}

// PathTo is the line from any entry back to the root, so a branch that was
// left behind can be read the same way as the one that was taken. The empty
// leaf is the root, and the path to it is no path at all.
func PathTo(entries []Entry, leaf string) []Entry {
	byID := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	at, ok := byID[leaf]
	if !ok {
		return nil
	}

	seen := map[string]bool{}
	var out []Entry
	for !seen[at.ID] {
		seen[at.ID] = true
		out = append(out, at)
		parent, ok := byID[at.Parent]
		if !ok {
			break
		}
		at = parent
	}

	slices.Reverse(out)
	return out
}

// Settings come from every entry — a clear should not lose your model —
// while messages come from the last clear onwards.
func Build(entries []Entry) Context {
	return build(Path(entries))
}

// BuildAt rebuilds the conversation as it stood at any entry in the tree,
// however many branches have grown past it since.
func BuildAt(entries []Entry, leaf string) Context {
	return build(PathTo(entries, leaf))
}

func build(entries []Entry) Context {
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
		c.Marks = append(c.Marks, "")
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
		if e.Type == KindTask && e.Task != nil {
			c.Tasks = append(c.Tasks, *e.Task)
			addUsage(&c.Usage, e.Task.Usage)
			continue
		}
		if e.Type != KindMessage || e.Message == nil {
			continue
		}
		c.Messages = append(c.Messages, *e.Message)
		c.Marks = append(c.Marks, e.ID)
		if e.Usage != nil {
			addUsage(&c.Usage, *e.Usage)
		}
	}
	return c
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.ThinkingTokens += u.ThinkingTokens
	total.CacheReadTokens += u.CacheReadTokens
	total.CacheWriteTokens += u.CacheWriteTokens
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
