package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type frameMsg struct{}

const (
	framesPerSecond = 60
	landingFrames   = 4
)

type motion struct {
	on        bool
	active    bool
	scrolling bool
	target    int
	landing   int

	// Where the bar is and where it is heading, in transcript lines.
	barAt int
	barTo int

	trail map[int]int
}

const barSteps = 6

// Something has started moving. The loop is not armed here: Update arms it
// after every message, so an animation cannot be lost by a caller that drops
// the command it was handed.
func (m *Model) animate() tea.Cmd { return nil }

// withFrame keeps one tick in flight for as long as anything is moving.
// Everything that animates shares it, so two things at once do not run at
// double rate.
func (m *Model) withFrame(cmd tea.Cmd) tea.Cmd {
	if !m.motion.on || m.motion.active || !m.animating() {
		return cmd
	}
	m.motion.active = true
	if cmd == nil {
		return frame()
	}
	return tea.Batch(cmd, frame())
}

// animating asks every part that can move whether it still is.
func (m *Model) animating() bool {
	switch {
	case m.motion.barAt != m.motion.barTo && m.motion.barAt >= 0:
	case m.motion.scrolling, m.motion.landing > 0, len(m.motion.trail) > 0:
	case m.chooser.open && (m.chooser.barAt != m.chooser.barTo ||
		len(m.chooser.trail) > 0 || m.chooser.fill < m.fillTarget()):
	case m.menu.open && (m.menu.barAt != m.menu.barTo || len(m.menu.trail) > 0):
	case m.picker.open && (m.picker.barAt != m.picker.barTo || len(m.picker.trail) > 0):
	case m.journey.open && (m.journey.barAt != m.journey.barTo || len(m.journey.trail) > 0):
	case m.agents.open && (m.agents.barAt != m.agents.barTo || len(m.agents.trail) > 0):
	default:
		return false
	}
	return true
}

const framesPerShade = 6

// What a mark loses for each line the bar puts between them. Small, so a mark
// fades mostly on the clock and is seen to slim where it was left; without any
// of it a long jump would lay one flat block of trail in a single frame.
const agePerLine = 2

func trailLife() int { return len(trail) * framesPerShade }

// The bar sits at a line of the transcript rather than at an entry, so it can
// be seen crossing the ground between two of them.
func (m *Model) barForLine(line int) string {
	switch {
	case line == m.motion.barAt && m.motion.landing > 0:
		return landedBar()
	case line == m.motion.barAt:
		return cursorBar()
	}
	if life, ok := m.motion.trail[line]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return noBar()
}

// The bar covers the distance in about the same time however far it is going:
// a step per frame across forty lines would be a journey, not an animation.
func (m *Model) advanceTranscriptBar() bool {
	if m.motion.barAt == m.motion.barTo || m.motion.barAt < 0 {
		return false
	}

	distance := m.motion.barTo - m.motion.barAt
	step := max(abs(distance)/barSteps, 1)
	if distance < 0 {
		step = -step
	}

	for range abs(step) {
		if m.motion.barAt == m.motion.barTo {
			break
		}
		m.leaveTrail(m.motion.barAt)
		m.motion.barAt += sign(step)
	}
	return m.motion.barAt != m.motion.barTo
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

func (m *Model) UseAnimation(on bool) { m.motion.on = on }
func frame() tea.Cmd {
	return tea.Tick(time.Second/framesPerSecond, func(time.Time) tea.Msg { return frameMsg{} })
}

// A mark starts fading the moment it is left, rather than being held at full
// width until the bar stops. Distance still counts for something, so a jump of
// forty lines tapers instead of landing as one solid block.
func (m *Model) leaveTrail(from int) {
	if !m.motion.on || from < 0 {
		return
	}
	if m.motion.trail == nil {
		m.motion.trail = map[int]int{}
	}

	for i, life := range m.motion.trail {
		if life -= agePerLine; life <= 0 {
			delete(m.motion.trail, i)
			continue
		}
		m.motion.trail[i] = life
	}
	m.motion.trail[from] = trailLife()
}

func (m *Model) step() tea.Cmd {
	working := m.fillMeter()

	if m.advanceTranscriptBar() {
		working = true
	}
	if m.advancePickerBar() {
		working = true
	}
	if m.advanceJourneyBar() {
		working = true
	}
	if m.advanceAgentsBar() {
		working = true
	}
	if m.advanceMenuBar() {
		working = true
	}
	if m.motion.landing > 0 {
		m.motion.landing--
	}
	for i := range m.motion.trail {
		m.motion.trail[i]--
		if m.motion.trail[i] <= 0 {
			delete(m.motion.trail, i)
		}
	}
	m.paint()

	// The viewport is only touched while a scroll is actually in flight;
	// otherwise something else animating would drag the transcript with it.
	if m.motion.scrolling {
		distance := m.motion.target - m.viewport.YOffset
		switch {
		case distance > 0:
			m.viewport.SetYOffset(m.viewport.YOffset + max(distance*2/3, 1))
		case distance < 0:
			m.viewport.SetYOffset(m.viewport.YOffset + min(distance*2/3, -1))
		}
		m.motion.scrolling = m.viewport.YOffset != m.motion.target
	}

	if m.motion.scrolling || m.motion.landing > 0 || len(m.motion.trail) > 0 {
		working = true
	}
	if !working {
		m.motion.active = false
		return nil
	}
	return frame()
}

// Only the transcript is rebuilt, not the entries: the bar lives between them
// now, so where it falls is a question about lines rather than about text.
func (m *Model) paint() {
	if m.ready {
		m.viewport.SetContent(m.transcript())
	}
}
