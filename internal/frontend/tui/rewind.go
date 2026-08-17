package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
)

func cmdRewind(m *Model, arg string) tea.Cmd {
	turns := agent.Turns(m.messages)
	if len(turns) == 0 {
		m.notice("nothing to go back to — you have not asked anything yet")
		return nil
	}

	if arg != "" {
		for _, t := range turns {
			if strings.HasPrefix(strings.ToLower(t.Prompt), strings.ToLower(arg)) {
				return m.rewind(t)
			}
		}
		m.push(entry{kind: entryError, text: fmt.Sprintf("no prompt of yours starts with %q", arg)})
		return nil
	}

	// Newest first: going back one turn is the common case, and the further
	// back you mean the longer you are willing to look for it.
	options := make([]choice, 0, len(turns))
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		dropped := len(m.messages) - t.At
		options = append(options, choice{
			label:  truncate(firstLine(t.Prompt), 40),
			value:  fmt.Sprint(t.At),
			detail: fmt.Sprintf("%d messages leave the context", dropped),
			body: []string{
				bodyStyle.Render(truncate(firstLine(t.Prompt), 60)),
				"",
				keyed("dropped", fmt.Sprintf("%d messages stop being sent", dropped)),
				"",
				hintStyle.Render("The prompt comes back to the composer to be changed and asked again."),
				hintStyle.Render("What came after stays in the session file, on a branch of its own."),
			},
		})
	}

	return m.ask(chooser{title: "rewind · take the conversation up from an earlier turn",
		options: options,
		apply: func(m *Model, picked choice) {
			for _, t := range agent.Turns(m.messages) {
				if fmt.Sprint(t.At) == picked.value {
					m.runCmd(m.rewind(t))
					return
				}
			}
		}})
}

func (m *Model) rewind(t agent.Turn) tea.Cmd {
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}
