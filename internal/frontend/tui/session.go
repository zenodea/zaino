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
	m.entries = nil
	m.rendered = nil

	for _, msg := range c.Messages {
		m.transcribe(msg)
	}
	m.rerender()
	m.syncViewport()
}

func (m *Model) transcribe(msg llm.Message) {
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
			m.entries = append(m.entries, entry{kind: kind, text: b.Text})

		case llm.ToolUseBlock:
			m.entries = append(m.entries, entry{
				kind:      entryTool,
				toolName:  b.Name,
				toolArgs:  compactArgs(b.Input, argsLimit),
				toolInput: string(b.Input),
				done:      true,
			})

		case llm.ToolResultBlock:
			m.closeRestoredTool(b)
		}
	}
}

func (m *Model) closeRestoredTool(result llm.ToolResultBlock) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind != entryTool || m.entries[i].resultLen > 0 {
			continue
		}
		m.entries[i].failed = result.IsError
		m.entries[i].resultLen = len(result.Content)
		m.entries[i].toolResult = result.Content
		return
	}
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
