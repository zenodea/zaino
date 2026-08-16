package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The bar column of every transcript line, top to bottom.
func gutter(m *Model) string {
	var b strings.Builder
	for _, line := range strings.Split(stripANSI(m.transcript()), "\n") {
		cell := " "
		if runes := []rune(line); len(runes) > 1 {
			cell = string(runes[1])
		}
		if !strings.ContainsRune("▌▍▎▏", []rune(cell)[0]) {
			cell = " "
		}
		b.WriteString(cell)
	}
	return b.String()
}

// The bar walks the lines between two entries, marking each as it passes.
func TestCursorLeavesATrail(t *testing.T) {
	m := longChat(t, 10)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.moveCursor(-3)

	marks := 0
	for range 200 {
		marks = max(marks, strings.Count(gutter(m), "▌")+
			strings.Count(gutter(m), "▍")+strings.Count(gutter(m), "▎"))
		if _, cmd := m.Update(frameMsg{}); cmd == nil {
			break
		}
	}
	if marks < 2 {
		t.Errorf("only %d marks were ever on screen; the bar is not leaving a trail", marks)
	}
}

// A tail that fades on a clock alone is a row of identical marks; it has to
// thin with distance too.
func TestTrailTapersByDistance(t *testing.T) {
	m := longChat(t, 12)
	m.UseAnimation(true)
	m.moveCursor(-1)
	m.moveCursor(-4)

	shapes := map[rune]bool{}
	for range 200 {
		for _, r := range gutter(m) {
			if r != ' ' {
				shapes[r] = true
			}
		}
		if _, cmd := m.Update(frameMsg{}); cmd == nil {
			break
		}
	}
	if len(shapes) < 3 {
		t.Errorf("the tail only ever took %d shapes: %v", len(shapes), shapes)
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

	// Update arms the loop after every message; these tests move the cursor
	// directly, so they arm it the same way.
	eased.withFrame(nil)

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

	m.withFrame(nil)

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

// Two full bars on screen read as two cursors rather than as one that has just
// moved, so the tail never borrows the cursor's own glyph.
func TestTheTailIsNeverMistakenForTheCursor(t *testing.T) {
	for _, glyph := range trail {
		if glyph.glyph == "▌" {
			t.Errorf("the tail uses %q, which is the cursor's own mark", glyph.glyph)
		}
	}

	m := longChat(t, 12)
	m.UseAnimation(true)
	m.moveCursor(-1)
	m.moveCursor(-4)

	for range 200 {
		if got := strings.Count(gutter(m), "▌"); got > 1 {
			t.Fatalf("%d full bars on screen at once: %q", got, gutter(m))
		}
		if _, cmd := m.Update(frameMsg{}); cmd == nil {
			break
		}
	}
}

// The loop used to be armed by whoever started an animation, so a caller that
// dropped the command it was handed left the frame loop dead for good.
func TestADroppedCommandDoesNotKillTheLoop(t *testing.T) {
	m := longChat(t, 30)
	m.UseAnimation(true)

	// toggleSelected returns a bool and throws its animation command away.
	m.moveCursor(-1)
	m.withFrame(nil)
	for m.motion.active {
		m.step()
	}
	m.toggleSelected()

	m.moveCursor(-1)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK}); cmd == nil && !m.motion.active {
		t.Fatal("the frame loop never restarted")
	}

	frames := 0
	for m.motion.active && frames < 300 {
		m.step()
		frames++
	}
	if frames == 0 {
		t.Error("nothing animated after a command was dropped")
	}
}

func TestTheLoopArmsItselfThroughUpdate(t *testing.T) {
	m := longChat(t, 30)
	m.UseAnimation(true)

	if m.motion.active {
		t.Fatal("the loop is running before anything moved")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})

	if !m.motion.active {
		t.Error("a key that moves the cursor did not arm the loop")
	}
}

// One tick at a time, however many things are moving.
func TestOnlyOneFrameIsEverInFlight(t *testing.T) {
	m := longChat(t, 30)
	m.UseAnimation(true)
	m.moveCursor(-5)

	first := m.withFrame(nil)
	second := m.withFrame(nil)
	if first == nil {
		t.Fatal("the first arm produced no tick")
	}
	if second != nil {
		t.Error("a second tick was scheduled while one was already in flight")
	}
}

func TestNothingAnimatesWhenAnimationIsOff(t *testing.T) {
	m := longChat(t, 30)
	m.UseAnimation(false)
	m.moveCursor(-5)

	if cmd := m.withFrame(nil); cmd != nil {
		t.Error("a tick was scheduled with animation off")
	}
	if m.motion.active {
		t.Error("the loop is marked running with animation off")
	}
}

// shadeAt reports how wide the trail is at a line: 3 is freshest, 0 is gone.
func shadeAt(m *Model, line int) int {
	glyph := m.barForLine(line)
	for i, t := range trail {
		if strings.Contains(glyph, t.glyph) {
			return len(trail) - i
		}
	}
	return 0
}

// The trail used to be a rigid comb dragged along by the bar: every mark died
// as fast as the bar moved, so nothing was seen to fade until it stopped.
func TestTheTrailFadesWhileTheBarIsStillMoving(t *testing.T) {
	m := longChat(t, 40)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.withFrame(nil)
	for m.motion.active {
		m.step()
	}

	from := m.motion.barAt
	m.moveCursor(-12)
	m.withFrame(nil)
	if m.motion.barAt == m.motion.barTo {
		t.Skip("the move did not travel")
	}

	m.step()
	first := shadeAt(m, from)
	if first == 0 {
		t.Fatalf("line %d was left with no mark at all", from)
	}

	faded := false
	for range 40 {
		if m.motion.barAt == m.motion.barTo {
			break
		}
		m.step()
		if shadeAt(m, from) < first {
			faded = true
			break
		}
	}
	if !faded {
		t.Errorf("the mark at line %d held shade %d for the whole journey", from, first)
	}
}

func TestTheTrailDisappearsWhileTheBarIsStillMoving(t *testing.T) {
	m := longChat(t, 60)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.withFrame(nil)
	for m.motion.active {
		m.step()
	}

	from := m.motion.barAt
	m.moveCursor(-30)
	m.withFrame(nil)

	for range 60 {
		if m.motion.barAt == m.motion.barTo {
			t.Skip("the bar arrived before the mark expired")
		}
		m.step()
		if shadeAt(m, from) == 0 {
			return
		}
	}
	t.Errorf("the mark at line %d outlived the whole journey", from)
}

// However far it jumps, the tail stays a tail rather than painting the screen.
func TestALongJumpLeavesABoundedTail(t *testing.T) {
	m := longChat(t, 80)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.withFrame(nil)
	for m.motion.active {
		m.step()
	}

	m.moveCursor(-50)
	m.withFrame(nil)

	worst := 0
	for range 80 {
		if !m.motion.active {
			break
		}
		m.step()
		worst = max(worst, len(m.motion.trail))
	}
	if worst == 0 {
		t.Fatal("the jump left no trail at all")
	}
	if worst > 4*len(trail) {
		t.Errorf("the tail grew to %d marks, want a tail and not a stripe", worst)
	}
}

// A tail that is all one shade is a block, not a comet.
func TestTheTailTapers(t *testing.T) {
	m := longChat(t, 60)
	m.UseAnimation(true)

	m.moveCursor(-1)
	m.withFrame(nil)
	for m.motion.active {
		m.step()
	}

	m.moveCursor(-30)
	m.withFrame(nil)
	for range 6 {
		m.step()
	}

	shades := map[int]bool{}
	for line := m.motion.barAt; line <= m.motion.barAt+80; line++ {
		if s := shadeAt(m, line); s > 0 {
			shades[s] = true
		}
	}
	if len(shades) < 2 {
		t.Errorf("the tail showed %d shade(s), want it to taper", len(shades))
	}
}
