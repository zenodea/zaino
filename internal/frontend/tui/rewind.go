package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func cmdRewind(m *Model, arg string) tea.Cmd {
	turns := agent.Turns(m.messages)
	if len(turns) == 0 {
		m.notice("nothing to go back to — you have not asked anything yet")
		return nil
	}

	// One step back is the common case, so that is what the bare command
	// does; a prefix names a turn further up, and /journey shows them all.
	if arg == "" {
		return m.rewind(turns[len(turns)-1])
	}
	for _, t := range turns {
		if strings.HasPrefix(strings.ToLower(t.Prompt), strings.ToLower(arg)) {
			return m.rewind(t)
		}
	}
	m.push(entry{kind: entryError, text: fmt.Sprintf("no prompt of yours starts with %q", arg)})
	return nil
}

// rewind is a journey of one step: with a session on disk it travels the
// same way /journey does, so the model and the rest of the settings come
// back to what they were then; without one it can only trim the messages.
func (m *Model) rewind(t agent.Turn) tea.Cmd {
	if stop, ok := m.stopFor(t); ok {
		m.takeUp(stop, "rewound")
		return nil
	}

	if err := m.rec.Rewind(t.At); err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}

	dropped := len(m.messages) - t.At
	m.messages = m.messages[:t.At]
	m.sessionUsage = llm.Usage{}
	m.showRecalled(t.Prompt)

	m.push(entry{kind: entryNotice, text: fmt.Sprintf(
		"rewound · %d messages left the context, and the prompt is back in the composer", dropped)})
	return nil
}

// The stop on the map that a turn in the context is.
func (m *Model) stopFor(t agent.Turn) (journeyStop, bool) {
	store := m.rec.Store()
	if store == nil {
		return journeyStop{}, false
	}
	entries, err := store.Entries()
	if err != nil {
		return journeyStop{}, false
	}
	marks := session.Build(entries).Marks
	if t.At >= len(marks) || marks[t.At] == "" {
		return journeyStop{}, false
	}
	for _, e := range entries {
		if e.ID == marks[t.At] {
			return journeyStop{id: e.ID, parent: e.Parent, prompt: t.Prompt, at: e.At}, true
		}
	}
	return journeyStop{}, false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}
