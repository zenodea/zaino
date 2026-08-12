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
