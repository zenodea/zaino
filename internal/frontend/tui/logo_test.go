package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPackArtIsRectangular(t *testing.T) {
	if len(packArt)%2 != 0 {
		t.Fatalf("packArt has %d rows, want an even number — cells are two pixels tall", len(packArt))
	}

	width := len(packArt[0])
	for i, row := range packArt {
		if len(row) != width {
			t.Errorf("row %d is %d wide, want %d", i, len(row), width)
		}
	}
}

func TestPackArtUsesKnownColours(t *testing.T) {
	palette := packPalette()
	for i, row := range packArt {
		for x := range len(row) {
			if row[x] == '.' {
				continue
			}
			if _, ok := palette[row[x]]; !ok {
				t.Errorf("row %d column %d uses %q, which has no colour", i, x, row[x])
			}
		}
	}
}

func TestLogoRenders(t *testing.T) {
	lines := strings.Split(logo(), "\n")
	if len(lines) != len(packArt)/2 {
		t.Errorf("logo is %d rows, want %d", len(lines), len(packArt)/2)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != len(packArt[0]) {
			t.Errorf("row %d is %d wide, want %d", i, w, len(packArt[0]))
		}
	}
}

func splashLines(m *Model) []string {
	var out []string
	for _, line := range strings.Split(m.splash(), "\n") {
		if strings.TrimSpace(stripANSI(line)) != "" {
			out = append(out, stripANSI(line))
		}
	}
	return out
}

// An empty screen has nothing else to hang the splash on, so it sits in the
// middle of both axes.
func TestTheSplashIsCentred(t *testing.T) {
	for _, size := range [][2]int{{92, 26}, {70, 30}, {120, 40}} {
		m := newTestModel(t, size[0], size[1])
		m.resize(size[0], size[1])

		// Measured from what can be seen: the logo's transparent cells are
		// spaces, and counting them as content would say it is off-centre.
		for _, line := range splashLines(m) {
			left := lipgloss.Width(line) - lipgloss.Width(strings.TrimLeft(line, " "))
			right := m.contentWidth() - lipgloss.Width(strings.TrimRight(line, " "))
			if diff := left - right; diff > 1 || diff < -1 {
				t.Errorf("at %dx%d a line sits %d from the left and %d from the right: %q",
					size[0], size[1], left, right, line)
			}
		}
	}
}

func TestTheSplashSitsInTheMiddleOfTheScreen(t *testing.T) {
	m := newTestModel(t, 92, 30)
	m.resize(92, 30)

	lines := strings.Split(m.splash(), "\n")
	above := 0
	for _, line := range lines {
		if strings.TrimSpace(stripANSI(line)) != "" {
			break
		}
		above++
	}
	if above < 2 {
		t.Errorf("only %d blank rows above the splash; it is not centred vertically", above)
	}
}

func TestTheSplashNeverOverflows(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 140} {
		m := newTestModel(t, width, 24)
		m.resize(width, 24)

		for _, line := range strings.Split(m.splash(), "\n") {
			if got := lipgloss.Width(line); got > m.contentWidth() {
				t.Errorf("at %d columns a splash line is %d wide, room is %d: %q",
					width, got, m.contentWidth(), stripANSI(line))
			}
		}
	}
}
