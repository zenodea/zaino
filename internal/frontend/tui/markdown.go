package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type span struct {
	text   string
	style  lipgloss.Style
	atomic bool
}

func renderMarkdown(text string, width int, base lipgloss.Style) string {
	if width < 8 {
		width = 8
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	var out []string
	var paragraph []string

	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		joined := strings.Join(paragraph, " ")
		out = append(out, wrapSpans(parseInline(joined, base), width, "", "")...)
		paragraph = nil
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if fence := fenceOf(trimmed); fence != "" {
			flush()
			body, next := collectFence(lines, i, fence)
			out = append(out, renderCode(body, width)...)
			i = next
			continue
		}

		if trimmed == "" {
			flush()
			out = append(out, "")
			continue
		}

		if isRule(trimmed) {
			flush()
			out = append(out, ruleStyle.Render(strings.Repeat("─", width)))
			continue
		}

		if level, heading, ok := headingOf(trimmed); ok {
			flush()
			style := mdHeadingStyle
			if level > 2 {
				style = mdSubHeadingStyle
			}
			out = append(out, wrapSpans(parseInline(heading, style), width, "", "")...)
			continue
		}

		if quote, ok := strings.CutPrefix(trimmed, "> "); ok {
			flush()
			out = append(out, wrapSpans(parseInline(quote, mdQuoteStyle), width-2,
				mdQuoteBar.Render("│")+" ", mdQuoteBar.Render("│")+" ")...)
			continue
		}

		if bullet, rest, ok := listItemOf(line); ok {
			flush()
			indent := strings.Repeat(" ", lipgloss.Width(bullet)+1)
			out = append(out, wrapSpans(parseInline(rest, base), width-lipgloss.Width(bullet)-1,
				mdBulletStyle.Render(bullet)+" ", indent)...)
			continue
		}

		paragraph = append(paragraph, trimmed)
	}
	flush()

	return strings.Join(trimTrailingBlanks(out), "\n")
}

func fenceOf(line string) string {
	for _, fence := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, fence) {
			return fence
		}
	}
	return ""
}

func collectFence(lines []string, start int, fence string) ([]string, int) {
	var body []string
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
			return body, i
		}
		body = append(body, lines[i])
	}
	return body, len(lines) - 1
}

func renderCode(body []string, width int) []string {
	out := make([]string, 0, len(body))
	for _, line := range body {
		out = append(out, mdCodeBlockStyle.Render("  "+clamp(line, max(width-2, 8))))
	}
	return out
}

func isRule(line string) bool {
	for _, mark := range []string{"---", "***", "___"} {
		if strings.Trim(line, strings.Trim(mark, "-*_")) == "" && strings.HasPrefix(line, mark) {
			return true
		}
	}
	return false
}

func headingOf(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(line[level:]), true
}

func listItemOf(line string) (bullet, rest string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	indent := strings.Repeat(" ", min(len(line)-len(trimmed), 8))

	for _, mark := range []string{"- ", "* ", "+ "} {
		if after, found := strings.CutPrefix(trimmed, mark); found {
			return indent + "•", after, true
		}
	}

	for i, r := range trimmed {
		if r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '.' || r == ')') && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
			return indent + trimmed[:i+1], trimmed[i+2:], true
		}
		break
	}
	return "", "", false
}

func trimTrailingBlanks(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func parseInline(s string, base lipgloss.Style) []span {
	var out []span
	var buf strings.Builder

	bold, italic, strike := false, false, false
	style := func() lipgloss.Style {
		st := base
		if bold {
			st = st.Bold(true)
		}
		if italic {
			st = st.Italic(true)
		}
		if strike {
			st = st.Strikethrough(true)
		}
		return st
	}
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, span{text: buf.String(), style: style()})
			buf.Reset()
		}
	}

	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "**"), strings.HasPrefix(s[i:], "__"):
			flush()
			bold = !bold
			i += 2

		case strings.HasPrefix(s[i:], "~~"):
			flush()
			strike = !strike
			i += 2

		case s[i] == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				flush()
				out = append(out, span{text: s[i+1 : i+1+end], style: mdCodeStyle, atomic: true})
				i += end + 2
				continue
			}
			buf.WriteByte(s[i])
			i++

		case s[i] == '*', s[i] == '_':
			if !emphasisAt(s, i) {
				buf.WriteByte(s[i])
				i++
				continue
			}
			flush()
			italic = !italic
			i++

		case s[i] == '[':
			text, url, width, ok := linkAt(s[i:])
			if !ok {
				buf.WriteByte(s[i])
				i++
				continue
			}
			flush()
			out = append(out, span{text: text, style: mdLinkStyle})
			out = append(out, span{text: " (" + url + ")", style: metaStyle})
			i += width

		default:
			buf.WriteByte(s[i])
			i++
		}
	}
	flush()
	return out
}

func emphasisAt(s string, i int) bool {
	if s[i] == '*' {
		return true
	}
	before := byte(' ')
	if i > 0 {
		before = s[i-1]
	}
	after := byte(' ')
	if i+1 < len(s) {
		after = s[i+1]
	}
	return !isWordByte(before) || !isWordByte(after)
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

func linkAt(s string) (text, url string, width int, ok bool) {
	close := strings.IndexByte(s, ']')
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return "", "", 0, false
	}
	end := strings.IndexByte(s[close:], ')')
	if end < 0 {
		return "", "", 0, false
	}
	return s[1:close], s[close+2 : close+end], close + end + 1, true
}

// Wrapping happens over the spans rather than the finished string, so a style
// that spans a line break survives it.
func wrapSpans(spans []span, width int, firstPrefix, restPrefix string) []string {
	if width < 4 {
		width = 4
	}

	var lines []string
	var line strings.Builder

	var run strings.Builder
	runStyle := lipgloss.NewStyle()

	used := 0
	prefix := firstPrefix

	// A separator is held back until a word actually follows it: afterwards it
	// sits inside escape sequences, where TrimRight cannot see it.
	pendingSpace := false

	flushRun := func() {
		if run.Len() > 0 {
			line.WriteString(runStyle.Render(run.String()))
			run.Reset()
		}
	}
	newline := func() {
		flushRun()
		lines = append(lines, prefix+line.String())
		line.Reset()
		used, prefix = 0, restPrefix
		pendingSpace = false
	}

	for _, sp := range spans {
		flushRun()
		runStyle = sp.style

		for _, word := range splitKeepingSpaces(sp.text, sp.atomic) {
			if word == "\n" {
				newline()
				runStyle = sp.style
				continue
			}
			if word == " " {
				if used > 0 {
					pendingSpace = true
				}
				continue
			}

			w := lipgloss.Width(word)
			gap := 0
			if pendingSpace {
				gap = 1
			}
			if used > 0 && used+gap+w > width {
				newline()
				runStyle = sp.style
				gap = 0
			}
			if used == 0 && w > width {
				word = clamp(word, width)
				w = lipgloss.Width(word)
			}
			if gap > 0 {
				run.WriteString(" ")
				used++
			}
			pendingSpace = false

			run.WriteString(word)
			used += w
		}
	}
	if run.Len() > 0 || line.Len() > 0 || len(lines) == 0 {
		newline()
	}
	return lines
}

func splitKeepingSpaces(s string, atomic bool) []string {
	if atomic {
		return []string{s}
	}
	var out []string
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\n' {
			out = append(out, string(s[i]))
			i++
			continue
		}
		j := strings.IndexAny(s[i:], " \n")
		if j < 0 {
			out = append(out, s[i:])
			break
		}
		out = append(out, s[i:i+j])
		i += j
	}
	return out
}
