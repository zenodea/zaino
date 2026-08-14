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
	trail     map[int]int
}

// Starts the frame loop if it is not already running. Everything that animates
// shares one tick, so two things moving at once do not run at double rate.
func (m *Model) animate() tea.Cmd {
	if !m.motion.on || m.motion.active {
		return nil
	}
	m.motion.active = true
	return frame()
}

const framesPerShade = 5

func trailLife() int { return len(trail) * framesPerShade }

func (m *Model) barFor(i int) string {
	switch {
	case i == m.cursor && m.motion.landing > 0:
		return landedBar()
	case i == m.cursor:
		return cursorBar()
	}
	if life, ok := m.motion.trail[i]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return noBar()
}

func (m *Model) UseAnimation(on bool) { m.motion.on = on }
func frame() tea.Cmd {
	return tea.Tick(time.Second/framesPerSecond, func(time.Time) tea.Msg { return frameMsg{} })
}

// Each step ages what is already behind you by a whole shade, so the tail
// tapers by distance rather than by clock.
func (m *Model) leaveTrail(from int) {
	if !m.motion.on || from < 0 {
		return
	}
	if m.motion.trail == nil {
		m.motion.trail = map[int]int{}
	}

	for i, life := range m.motion.trail {
		if life -= framesPerShade; life <= 0 {
			delete(m.motion.trail, i)
			m.paint(i)
			continue
		}
		m.motion.trail[i] = life
	}
	m.motion.trail[from] = trailLife()
}

func (m *Model) step() tea.Cmd {
	working := m.fillMeter()

	repaint := make([]int, 0, len(m.motion.trail)+1)
	if m.motion.landing > 0 {
		m.motion.landing--
		repaint = append(repaint, m.cursor)
	}
	for i := range m.motion.trail {
		m.motion.trail[i]--
		if m.motion.trail[i] <= 0 {
			delete(m.motion.trail, i)
		}
		repaint = append(repaint, i)
	}
	m.paint(repaint...)

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

func (m *Model) paint(indexes ...int) {
	if len(indexes) == 0 {
		return
	}
	for _, i := range indexes {
		if i < 0 || i >= len(m.rendered) {
			continue
		}
		m.rendered[i] = m.entries[i].renderAs(m.contentWidth(), m.barFor(i))
	}
	m.viewport.SetContent(m.transcript())
}
