package agent

import (
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

// A Turn is a prompt of your own, and where it sits in the conversation. It
// is the only place a conversation can be taken up again from: going back to
// anything else would leave a tool call without its result, or a result
// without its call.
type Turn struct {
	At     int
	Prompt string
}

func Turns(messages []llm.Message) []Turn {
	var out []Turn
	for i, msg := range messages {
		if msg.Role != llm.RoleUser || hasToolResult(msg) {
			continue
		}
		// The summary a compaction leaves is said in your voice, but it is not
		// a turn: it is the boundary a rewind cannot cross.
		text := strings.TrimSpace(msg.Text())
		if text == "" || strings.HasPrefix(text, SummaryPrefix) {
			continue
		}
		out = append(out, Turn{At: i, Prompt: text})
	}
	return out
}

func hasToolResult(msg llm.Message) bool {
	for _, block := range msg.Content {
		if _, ok := block.(llm.ToolResultBlock); ok {
			return true
		}
	}
	return false
}
