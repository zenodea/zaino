package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func gutter(m *Model) string {
	var b strings.Builder
	for i := range m.entries {
		bar := stripANSI(m.barFor(i))
		if bar == "" {
			bar = " "
		}
		b.WriteString(bar)
	}
	return b.String()
}

func TestCursorLeavesATrail(t *testing.T) {
	m := longChat(t, 10)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.moveCursor(-1)

	if got := strings.TrimRight(gutter(m), " "); !strings.HasSuffix(got, "▌▌") {
		t.Errorf("gutter = %q, want a mark left where the cursor came from", got)
	}
}

func TestTrailTapersByDistance(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(true)
	for range 4 {
		m.moveCursor(-1)
	}

	marks := []rune(strings.TrimRight(gutter(m), " "))
	if len(marks) < 4 {
		t.Fatalf("gutter = %q, want at least four marks", string(marks))
	}

	tail := marks[len(marks)-4:]
	seen := map[rune]bool{}
	for _, r := range tail {
		seen[r] = true
	}
	if len(seen) < 3 {
		t.Errorf("tail = %q, want the marks to thin out behind the cursor", string(tail))
	}
}

func TestTrailFadesAway(t *testing.T) {
	m := longChat(t, 10)
	m.UseAnimation(true)
	m.moveCursor(-1)
	m.moveCursor(-1)

	for range 200 {
		if !m.motion.active {
			break
		}
		m.step()
	}

	if len(m.motion.trail) != 0 {
		t.Errorf("%d marks are still lit after the animation settled", len(m.motion.trail))
	}
	if got := strings.Count(strings.TrimRight(gutter(m), " "), "▌"); got != 1 {
		t.Errorf("gutter = %q, want only the cursor left", strings.TrimRight(gutter(m), " "))
	}
}

func TestNoTrailWithAnimationOff(t *testing.T) {
	m := longChat(t, 10)
	m.UseAnimation(false)
	m.moveCursor(-1)
	m.moveCursor(-1)

	if len(m.motion.trail) != 0 {
		t.Errorf("a trail was left with animation turned off: %v", m.motion.trail)
	}
}

func TestAnimationEasesToTheSameOffset(t *testing.T) {
	still := longChat(t, 12)
	still.UseAnimation(false)

	eased := longChat(t, 12)
	eased.UseAnimation(true)

	for range 8 {
		still.moveCursor(-1)
		eased.moveCursor(-1)
	}

	if eased.viewport.YOffset == still.viewport.YOffset {
		t.Error("the animated view arrived instantly; nothing was eased")
	}

	frames := 0
	for eased.motion.active {
		if frames++; frames > 200 {
			t.Fatal("the animation never settled")
		}
		eased.step()
	}
	if eased.viewport.YOffset != still.viewport.YOffset {
		t.Errorf("eased to %d, want %d", eased.viewport.YOffset, still.viewport.YOffset)
	}
}

func TestAnimationDecelerates(t *testing.T) {
	m := longChat(t, 30)
	m.UseAnimation(true)
	for range 25 {
		m.moveCursor(-1)
	}

	var steps []int
	previous := m.viewport.YOffset
	for m.motion.active && len(steps) < 200 {
		m.step()
		if d := previous - m.viewport.YOffset; d > 0 {
			steps = append(steps, d)
		}
		previous = m.viewport.YOffset
	}

	if len(steps) < 3 {
		t.Fatalf("only %d moving frames, expected an eased run", len(steps))
	}
	if steps[0] <= steps[len(steps)-1] {
		t.Errorf("steps went %v, want them to shrink as it arrives", steps)
	}
}

func TestAnimationOffIsInstant(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(false)
	for range 8 {
		m.moveCursor(-1)
	}
	if m.motion.active {
		t.Error("an animation started with animation turned off")
	}
}

func TestTrailKeepsFadingThroughTheUpdateLoop(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd == nil {
		t.Fatal("moving the cursor did not schedule a frame")
	}
	if _, ok := cmd().(frameMsg); !ok {
		t.Fatalf("scheduled %T, want a frame", cmd())
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if len(m.motion.trail) == 0 {
		t.Fatal("no trail was left behind")
	}

	// One press, then nothing but frames: the tail has to fade on its own.
	seen := map[string]bool{}
	for range 200 {
		_, cmd = m.Update(frameMsg{})
		seen[gutter(m)] = true
		if cmd == nil {
			break
		}
	}

	if len(seen) < 3 {
		t.Errorf("the tail only took %d shapes while fading; it is not animating", len(seen))
	}
	if len(m.motion.trail) != 0 {
		t.Errorf("the tail never faded out: %v", m.motion.trail)
	}
	if m.motion.active {
		t.Error("the frame loop is still running with nothing left to animate")
	}
}
