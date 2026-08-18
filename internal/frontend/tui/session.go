package tui

import (
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/recall"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/wirelog"
)

func (m *Model) UseRecall(l *recall.List) { m.recall = l }

func (m *Model) UseSession(repo session.Repo, rec *session.Recorder) {
	m.repo = repo
	if rec != nil {
		m.rec = rec
	}
}

func (m *Model) UseWireLog(w *wirelog.Log) { m.wire = w }

func (m *Model) Restore(c session.Context) {
	m.messages = c.Messages
	m.sessionUsage = c.Usage
	m.entries = transcribeMessages(c.Messages)
	m.rendered = nil
	m.rebuildTasks(c.Tasks)

	m.rerender()
	m.syncViewport()
}

func transcribeMessages(msgs []llm.Message) []entry {
	var entries []entry
	for _, msg := range msgs {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case llm.TextBlock:
				if strings.TrimSpace(b.Text) == "" {
					continue
				}
				kind := entryAssistant
				if msg.Role == llm.RoleUser {
					kind = entryUser
				}
				entries = append(entries, entry{kind: kind, text: b.Text})

			case llm.ToolUseBlock:
				entries = append(entries, entry{
					kind:      entryTool,
					toolName:  b.Name,
					toolID:    b.ID,
					toolArgs:  compactArgs(b.Input, argsLimit),
					toolInput: string(b.Input),
					done:      true,
				})

			case llm.ToolResultBlock:
				closeRestoredTool(entries, b)
			}
		}
	}
	return entries
}

func closeRestoredTool(entries []entry, result llm.ToolResultBlock) {
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.kind != entryTool || e.resultLen > 0 {
			continue
		}
		if e.toolID != "" && result.ToolUseID != "" && e.toolID != result.ToolUseID {
			continue
		}
		entries[i].failed = result.IsError
		entries[i].resultLen = len(result.Content)
		entries[i].toolResult = result.Content
		return
	}
}

func (m *Model) compacted(msg compactMsg) {
	m.messages = msg.Kept
	m.saveError(m.rec.Compact(msg.Summary, msg.Kept))
	m.notice("compacted · %d messages kept", max(len(msg.Kept)-1, 0))
}

func (m *Model) record(n session.New) {
	m.saveError(m.rec.Append(n))
}

func (m *Model) saveError(err error) {
	if err == nil || m.saveFailed {
		return
	}
	m.saveFailed = true
	m.push(entry{kind: entryError, text: "session not being saved: " + err.Error()})
}

func (m *Model) sessionID() string { return m.rec.ID() }
