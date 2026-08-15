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

	toolInput  string
	toolResult string
	expanded   bool
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
		if e.expanded {
			return "▾", toolMarker
		}
		return "▸", toolMarker
	case entryError:
		if e.folds() && !e.expanded {
			return "▸", errorMarker
		}
		if e.folds() {
			return "▾", errorMarker
		}
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

// Anything that runs past a line or two is shown by its first line until it is
// asked for: a three-line error with a bar down every line is a wall.
func (e entry) folds() bool {
	if e.kind != entryError && e.kind != entryNotice {
		return false
	}
	return strings.Contains(strings.TrimSpace(e.text), "\n")
}

func (e entry) firstLine() string {
	line, _, _ := strings.Cut(strings.TrimSpace(e.text), "\n")
	return line
}

func (e entry) render(width int) string { return e.renderAs(width, nil) }

// bars is keyed by line within this entry, so a bar travelling through a long
// entry can sit on any of its lines rather than only on the first.
func (e entry) renderAs(width int, bars map[int]string) string {
	marker, markerStyle := e.marker()
	bodyWidth := max(width-gutterWidth, 20)

	text := e.text
	switch {
	case e.kind == entryTool:
		text = e.toolLine(bodyWidth)
	case e.folds() && !e.expanded:
		text = e.firstLine()
	}
	if strings.TrimSpace(text) == "" {
		return ""
	}

	var wrapped string
	switch {
	case e.kind == entryAssistant:
		wrapped = renderMarkdown(text, bodyWidth, e.bodyStyle())
	case e.kind == entryTool:
		wrapped = e.bodyStyle().Render(clamp(text, bodyWidth))
	default:
		wrapped = e.bodyStyle().Width(bodyWidth).Render(text)
	}
	if e.kind == entryTool && e.expanded {
		wrapped += "\n" + e.detail(bodyWidth)
	}

	rest := strings.Repeat(" ", max(gutterWidth-lipgloss.Width(marker)-1, 0))

	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		bar, ok := bars[i]
		if !ok {
			bar = noBar()
		}
		if i == 0 {
			lines[i] = markerStyle.Render(marker) + bar + rest + line
			continue
		}
		lines[i] = " " + bar + rest + line
	}
	return strings.Join(lines, "\n")
}

func (e entry) detail(width int) string {
	var lines []string

	if input := e.toolInput; strings.TrimSpace(input) != "" && strings.TrimSpace(input) != "{}" {
		lines = append(lines, metaStyle.Render("input"))
		lines = append(lines, indentBlock(prettyJSON(strings.TrimSpace(input)), width, mdCodeStyle)...)
	}
	if result := e.toolResult; strings.TrimSpace(result) != "" {
		label, style := "result", toolStyle
		if e.failed {
			label, style = "error", errorStyle
		}
		lines = append(lines, metaStyle.Render(label))
		lines = append(lines, indentBlock(result, width, style)...)
	}
	if len(lines) == 0 {
		lines = append(lines, metaStyle.Render("(nothing to show)"))
	}
	return strings.Join(lines, "\n")
}

const detailLines = 40

func indentBlock(text string, width int, style lipgloss.Style) []string {
	raw := strings.Split(strings.TrimRight(text, "\n"), "\n")

	hidden := 0
	if len(raw) > detailLines {
		hidden = len(raw) - detailLines
		raw = raw[:detailLines]
	}

	// Styled per line, not per block: rendering the whole thing at once puts the
	// reset after the final newline and splits into a phantom last line. Tabs go
	// too — the terminal decides how far one reaches.
	out := make([]string, 0, len(raw)+1)
	for _, line := range raw {
		out = append(out, "  "+style.Render(clamp(strings.ReplaceAll(line, "\t", "    "), max(width-2, 8))))
	}
	if hidden > 0 {
		out = append(out, metaStyle.Render(fmt.Sprintf("  … %d more lines", hidden)))
	}
	return out
}

func prettyJSON(raw string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return raw
	}
	return buf.String()
}

func (e entry) toolLine(width int) string {
	left := fmt.Sprintf("%-6s %s", e.toolName, e.toolSummary())
	right := e.toolStatus()

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		left = clamp(left, max(width-lipgloss.Width(right)-2, 8))
		gap = max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (e entry) toolStatus() string {
	switch {
	case !e.done:
		return "…"
	case e.failed:
		return "failed"
	default:
		return humanBytes(e.resultLen)
	}
}

func (e entry) toolSummary() string {
	var args map[string]any
	if err := json.Unmarshal([]byte(e.toolInput), &args); err != nil || len(args) == 0 {
		return e.toolArgs
	}

	text := func(keys ...string) string {
		for _, key := range keys {
			if s, ok := args[key].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}

	switch main := text("path", "command", "pattern"); {
	case main == "":
		return e.toolArgs
	case e.toolName == "grep" || e.toolName == "find":
		if glob, ok := args["glob"].(string); ok && glob != "" {
			return fmt.Sprintf("%q in %s", main, glob)
		}
		return fmt.Sprintf("%q", main)
	default:
		return main
	}
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
