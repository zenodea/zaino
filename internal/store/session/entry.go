package session

import (
	"time"

	"github.com/zenodea/zaino/internal/llm"
)

type Kind string

const (
	KindMessage  Kind = "message"
	KindModel    Kind = "model"
	KindSystem   Kind = "system"
	KindThinking Kind = "thinking"
	KindEffort   Kind = "effort"
	KindLimit    Kind = "limit"
	KindClear    Kind = "clear"

	KindPermission Kind = "permission"
	KindCompact    Kind = "compact"
	KindTask       Kind = "task"
)

// TaskBody is a child agent's whole run, keyed by the task call's tool-use ID.
type TaskBody struct {
	ID          string        `json:"id"`
	Description string        `json:"description,omitempty"`
	Agent       string        `json:"agent,omitempty"`
	Model       string        `json:"model,omitempty"`
	Depth       int           `json:"depth,omitempty"`
	Failed      bool          `json:"failed,omitempty"`
	Messages    []llm.Message `json:"messages,omitempty"`
	Usage       llm.Usage     `json:"usage"`
}

// Unexported so entries can only be built through the constructors below,
// which is what keeps ID, Parent, Seq and At the store's business.
type body struct {
	Message *llm.Message `json:"message,omitempty"`
	Usage   *llm.Usage   `json:"usage,omitempty"`

	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Text     string `json:"text,omitempty"`
	Level    string `json:"level,omitempty"`
	On       *bool  `json:"on,omitempty"`

	// A pointer because a ceiling of none is a setting like any other, and
	// zero would read as never having been set.
	Tokens *int `json:"tokens,omitempty"`

	Tool     string `json:"tool,omitempty"`
	Action   string `json:"action,omitempty"`
	Decision string `json:"decision,omitempty"`

	Task *TaskBody `json:"task,omitempty"`
}

type New struct {
	Type Kind `json:"type"`
	body
}

type Entry struct {
	ID     string    `json:"id"`
	Parent string    `json:"parent,omitempty"`
	Seq    int       `json:"seq"`
	At     time.Time `json:"at"`
	Type   Kind      `json:"type"`
	body
}

func Message(m llm.Message, u *llm.Usage) New {
	return New{Type: KindMessage, body: body{Message: &m, Usage: u}}
}

func Model(providerName, model string) New {
	return New{Type: KindModel, body: body{Provider: providerName, Model: model}}
}

func System(text string) New {
	return New{Type: KindSystem, body: body{Text: text}}
}

func Thinking(on bool) New {
	return New{Type: KindThinking, body: body{On: &on}}
}

func Effort(level string) New {
	return New{Type: KindEffort, body: body{Level: level}}
}

func Limit(tokens int) New {
	return New{Type: KindLimit, body: body{Tokens: &tokens}}
}

func Clear() New {
	return New{Type: KindClear}
}

func Compacted(text string) New {
	return New{Type: KindCompact, body: body{Text: text}}
}

func Task(t TaskBody) New {
	return New{Type: KindTask, body: body{Task: &t}}
}

func Permission(tool, action, target, decision string) New {
	return New{Type: KindPermission, body: body{
		Tool: tool, Action: action, Text: target, Decision: decision,
	}}
}

func (e Entry) Prompt() string {
	if e.Type != KindMessage || e.Message == nil || e.Message.Role != llm.RoleUser {
		return ""
	}
	return e.Message.Text()
}
