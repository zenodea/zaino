package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent = lipgloss.AdaptiveColor{Light: "#A26B2C", Dark: "#D9A05B"}

	speech = lipgloss.AdaptiveColor{Light: "#3F6B22", Dark: "#9ECE6A"}

	tooling = lipgloss.AdaptiveColor{Light: "#1F6F94", Dark: "#7DCFFF"}

	muted = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B90A0"}

	subtle = lipgloss.AdaptiveColor{Light: "#D4D4D8", Dark: "#3B3F4A"}

	danger = lipgloss.AdaptiveColor{Light: "#B02A45", Dark: "#F7768E"}

	body = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6E6EA"}

	packDeep = lipgloss.AdaptiveColor{Light: "#7E5220", Dark: "#B8813F"}
	packLit  = lipgloss.AdaptiveColor{Light: "#C08A45", Dark: "#EFBE7C"}
	packEdge = lipgloss.AdaptiveColor{Light: "#5E3A14", Dark: "#8A5E28"}
)

var (
	brandStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	metaStyle  = lipgloss.NewStyle().Foreground(muted)
	ruleStyle  = lipgloss.NewStyle().Foreground(subtle)

	userMarker      = lipgloss.NewStyle().Foreground(accent).Bold(true)
	assistantMarker = lipgloss.NewStyle().Foreground(speech).Bold(true)
	toolMarker      = lipgloss.NewStyle().Foreground(tooling)
	thinkingMarker  = lipgloss.NewStyle().Foreground(muted)
	errorMarker     = lipgloss.NewStyle().Foreground(danger).Bold(true)

	bodyStyle     = lipgloss.NewStyle().Foreground(body)
	userStyle     = lipgloss.NewStyle().Foreground(body)
	thinkingStyle = lipgloss.NewStyle().Foreground(muted).Italic(true)
	toolStyle     = lipgloss.NewStyle().Foreground(tooling)
	errorStyle    = lipgloss.NewStyle().Foreground(danger)
	hintStyle     = lipgloss.NewStyle().Foreground(muted)
	spinnerStyle  = lipgloss.NewStyle().Foreground(accent)

	menuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	menuPickStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
)

var (
	mdHeadingStyle    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	mdSubHeadingStyle = lipgloss.NewStyle().Foreground(body).Bold(true)
	mdCodeStyle       = lipgloss.NewStyle().Foreground(tooling)
	mdCodeBlockStyle  = lipgloss.NewStyle().Foreground(tooling)
	mdQuoteStyle      = lipgloss.NewStyle().Foreground(muted).Italic(true)
	mdQuoteBar        = lipgloss.NewStyle().Foreground(subtle)
	mdLinkStyle       = lipgloss.NewStyle().Foreground(speech)
	mdBulletStyle     = lipgloss.NewStyle().Foreground(accent)
)

var (
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2F6B37", Dark: "#9ECE6A"})
	removeStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B02A45", Dark: "#F7768E"})
	gutterStyle = lipgloss.NewStyle().Foreground(subtle)

	askBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(0, 1)

	keyCapStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	packStyle   = lipgloss.NewStyle().Foreground(accent)

	manualChip = lipgloss.NewStyle().Foreground(muted)
	acceptChip = lipgloss.NewStyle().Foreground(speech).Bold(true)
	planChip   = lipgloss.NewStyle().Foreground(tooling).Bold(true)
	bypassChip = lipgloss.NewStyle().Foreground(danger).Bold(true)

	vimNormalChip = lipgloss.NewStyle().Foreground(accent).Bold(true)
	vimVisualChip = lipgloss.NewStyle().Foreground(speech).Bold(true)
	quitChip      = lipgloss.NewStyle().Foreground(danger).Bold(true)
	visualStyle   = lipgloss.NewStyle().Foreground(body).Background(subtle)
	selectedBar   = lipgloss.NewStyle().Foreground(accent)
	landingBar    = lipgloss.NewStyle().Foreground(packLit).Bold(true)
)

const gutterWidth = 3

var packArt = []string{
	".....SSSS.....",
	"....SS..SS....",
	"....SS..SS....",
	"..OOHHHHHHOO..",
	".OHHHHHHHHHHO.",
	"SOBBBBBBBBBBOS",
	"SOZZZZZZZZZZOS",
	"SOBBBBBBBBBBOS",
	"SOBBBBBBBBBBOS",
	"SOBOOOOOOOOBOS",
	"SOBOPPPPPPOBOS",
	"SOBOPZZZZPOBOS",
	"SOBOPPPPPPOBOS",
	"SOBOOOOOOOOBOS",
	".OBBBBBBBBBBO.",
	"..OOOOOOOOOO..",
}

func packPalette() map[byte]lipgloss.TerminalColor {
	return map[byte]lipgloss.TerminalColor{
		'S': muted,
		'B': accent,
		'H': packLit,
		'P': packDeep,
		'O': packEdge,
		'Z': subtle,
	}
}

func logo() string {
	palette := packPalette()
	rows := make([]string, 0, len(packArt)/2)

	for y := 0; y+1 < len(packArt); y += 2 {
		top, bottom := packArt[y], packArt[y+1]
		var b strings.Builder
		for x := range len(top) {
			upper, hasUpper := palette[top[x]]
			lower, hasLower := palette[bottom[x]]
			switch {
			case hasUpper && hasLower:
				b.WriteString(lipgloss.NewStyle().Foreground(upper).Background(lower).Render("▀"))
			case hasUpper:
				b.WriteString(lipgloss.NewStyle().Foreground(upper).Render("▀"))
			case hasLower:
				b.WriteString(lipgloss.NewStyle().Foreground(lower).Render("▄"))
			default:
				b.WriteString(" ")
			}
		}
		rows = append(rows, b.String())
	}
	return strings.Join(rows, "\n")
}

func brandMark() string { return packStyle.Render("▟▙") }

func rule(width int) string {
	if width <= 0 {
		return ""
	}
	line := make([]rune, width)
	for i := range line {
		line[i] = '─'
	}
	return ruleStyle.Render(string(line))
}

var trail = []struct {
	glyph string
	style lipgloss.Style
}{
	{"▌", lipgloss.NewStyle().Foreground(packDeep)},
	{"▍", lipgloss.NewStyle().Foreground(packDeep)},
	{"▎", lipgloss.NewStyle().Foreground(muted)},
	{"▏", lipgloss.NewStyle().Foreground(subtle)},
}

func trailBar(life int) string {
	step := len(trail) - life
	if step < 0 || step >= len(trail) {
		return " "
	}
	return trail[step].style.Render(trail[step].glyph)
}

func cursorBar() string { return selectedBar.Render("▌") }
func landedBar() string { return landingBar.Render("▌") }
func noBar() string     { return " " }
