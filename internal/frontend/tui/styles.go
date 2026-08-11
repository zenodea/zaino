package tui

import "github.com/charmbracelet/lipgloss"

var (
	accent = lipgloss.AdaptiveColor{Light: "#A26B2C", Dark: "#D9A05B"}

	speech = lipgloss.AdaptiveColor{Light: "#3F6B22", Dark: "#9ECE6A"}

	tooling = lipgloss.AdaptiveColor{Light: "#1F6F94", Dark: "#7DCFFF"}

	muted = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B90A0"}

	subtle = lipgloss.AdaptiveColor{Light: "#D4D4D8", Dark: "#3B3F4A"}

	danger = lipgloss.AdaptiveColor{Light: "#B02A45", Dark: "#F7768E"}

	body = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6E6EA"}
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

const gutterWidth = 3

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
