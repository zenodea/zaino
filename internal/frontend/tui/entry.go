package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryThinking
	entryTool
	entryError
	entryNotice
)

type entry struct {
	kind entryKind
	text string

	toolName  string
	toolArgs  string
	done      bool
	failed    bool
	resultLen int
}

func (e entry) marker() (string, lipgloss.Style) {
	switch e.kind {
	case entryUser:
		return "›", userMarker
	case entryAssistant:
		return "◆", assistantMarker
	case entryThinking:
		return "⋯", thinkingMarker
	case entryTool:
		return "·", toolMarker
	case entryError:
		return "✗", errorMarker
	default:
		return " ", metaStyle
	}
}

func (e entry) bodyStyle() lipgloss.Style {
	switch e.kind {
	case entryUser:
		return userStyle
	case entryThinking:
		return thinkingStyle
	case entryTool:
		return toolStyle
	case entryError:
		return errorStyle
	case entryNotice:
		return hintStyle
	default:
		return bodyStyle
	}
}

func (e entry) render(width int) string {
	text := e.text
	if e.kind == entryTool {
		text = e.toolLine()
	}
	if strings.TrimSpace(text) == "" {
		return ""
	}

	bodyWidth := max(width-gutterWidth, 20)
	wrapped := e.bodyStyle().Width(bodyWidth).Render(text)

	marker, markerStyle := e.marker()
	pad := strings.Repeat(" ", gutterWidth)
	head := markerStyle.Render(marker) + strings.Repeat(" ", gutterWidth-lipgloss.Width(marker))

	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		if i == 0 {
			lines[i] = head + line
		} else {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

func (e entry) toolLine() string {
	var b strings.Builder
	b.WriteString(e.toolName)
	if e.toolArgs != "" && e.toolArgs != "{}" {
		b.WriteString(" ")
		b.WriteString(e.toolArgs)
	}
	switch {
	case !e.done:
		b.WriteString(" …")
	case e.failed:
		b.WriteString(" → failed")
	default:
		b.WriteString(fmt.Sprintf(" → ok, %s", humanBytes(e.resultLen)))
	}
	return b.String()
}

func compactArgs(raw json.RawMessage, limit int) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return truncate(string(raw), limit)
	}
	return truncate(buf.String(), limit)
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fkB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

func humanTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}
