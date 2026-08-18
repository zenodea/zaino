package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/store/session"
)

// A stop is one turn of yours drawn on the map: a place the conversation
// passed through, and a place it can be taken up again from.
type journeyStop struct {
	id     string
	parent string
	prompt string
	line   int
	tip    bool
	onPath bool
	here   bool
	at     time.Time

	// What the signpost says: the context branching here would rebuild, the
	// turns further down this road, and when it was last travelled.
	messages int
	tokens   int
	beyond   int
	latest   time.Time
}

type journeyLineKind int

const (
	lineRoad journeyLineKind = iota
	lineStop
	lineLandmark
)

// A line of the map. Roads and landmarks are ground between stops: the bar
// travels over them, the cursor never rests on them.
type journeyLine struct {
	kind journeyLineKind
	road string // the glyphs, paved with weight and colour at render time
	stop int    // index into stops, for lineStop
	text string // the label, for lineLandmark
}

// The journey view is the session file drawn as the map it is: every turn on
// every road, the abandoned ones included, and any stop a place to start
// again from. The road you took bends at each divergence — the oldest attempt
// keeps its lane, and what replaced it curves out — so how winding the map
// looks is how winding the conversation was.
type journey struct {
	open     bool
	lines    []journeyLine
	stops    []journeyStop
	branches int
	cursor   int

	// The picker's travelling bar, for the same reason the picker has one.
	barAt int
	barTo int
	trail map[int]int
}

func cmdJourney(m *Model, _ string) tea.Cmd {
	store := m.rec.Store()
	if store == nil {
		m.notice("no session on disk — /rewind still walks the live conversation")
		return nil
	}
	entries, err := store.Entries()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}
	leaf, err := store.Leaf()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return nil
	}

	lines, stops, branches := draw(entries, session.PathTo(entries, leaf))
	if len(stops) == 0 {
		m.notice("nothing to look back on — you have not asked anything yet")
		return nil
	}
	onPath := pathSet(entries, leaf)
	for i := range stops {
		tip := exchangeTip(entries, stops[i].id, onPath)
		stops[i].messages = len(session.BuildAt(entries, tip).Messages)
		stops[i].tokens = contextAt(entries, tip)
	}

	m.journey = journey{open: true, lines: lines, stops: stops, branches: branches,
		barAt: -1, barTo: -1}
	for i, s := range stops {
		if s.here {
			m.journey.cursor = i
			break
		}
	}
	_, cursor := m.journeyWindow(m.journeyRowCount())
	m.journey.barAt, m.journey.barTo = cursor, cursor
	m.syncViewport()
	return nil
}

// draw lays the tree out as roads. A turn with one follow-up continues
// straight down its lane; at a fork the oldest branch keeps the lane and each
// later one curves out to a lane of its own, the taken road drawn heavy and
// the left ones dashed.
func draw(entries []session.Entry, path []session.Entry) ([]journeyLine, []journeyStop, int) {
	byID := make(map[string]session.Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	onPath := make(map[string]bool, len(path))
	for _, e := range path {
		onPath[e.ID] = true
	}

	b := &roads{byID: byID, onPath: onPath}
	for i, root := range session.Tree(entries) {
		if i > 0 {
			b.lines = append(b.lines, journeyLine{kind: lineRoad, stop: -1})
		}
		b.walk(root, "", "", "", "", true)
	}

	here := ""
	for i := range b.stops {
		if b.stops[i].onPath {
			here = b.stops[i].id
		}
		if b.stops[i].tip {
			b.branches++
		}
	}
	for i := range b.stops {
		b.stops[i].here = b.stops[i].id == here
	}
	return b.lines, b.stops, b.branches
}

type roads struct {
	byID     map[string]session.Entry
	onPath   map[string]bool
	lines    []journeyLine
	stops    []journeyStop
	branches int
}

// walk draws one stop and everything down the road from it. stopRow is the
// ground the stop sits on — its lane, or its lane plus the curve out of its
// parent's — while aboveLane and aboveVert pave the road rows leading in.
func (b *roads) walk(n *session.Node, lane, stopRow, aboveLane, aboveVert string,
	root bool) (beyond int, latest time.Time) {
	if !root {
		b.lines = append(b.lines, journeyLine{kind: lineRoad, road: aboveLane + aboveVert, stop: -1})
		for _, mark := range b.landmarks(n.Entry) {
			b.lines = append(b.lines,
				journeyLine{kind: lineLandmark, road: aboveLane, text: mark, stop: -1},
				journeyLine{kind: lineRoad, road: aboveLane + aboveVert, stop: -1})
		}
	}

	at := len(b.stops)
	b.stops = append(b.stops, journeyStop{
		id:     n.Entry.ID,
		parent: n.Entry.Parent,
		prompt: n.Entry.Prompt(),
		line:   len(b.lines),
		tip:    len(n.Children) == 0,
		onPath: b.onPath[n.Entry.ID],
		at:     n.Entry.At,
	})
	b.lines = append(b.lines, journeyLine{kind: lineStop, road: stopRow, stop: at})

	latest = n.Entry.At
	took := func(sub int, when time.Time) {
		beyond += sub + 1
		if when.After(latest) {
			latest = when
		}
	}

	if len(n.Children) == 1 {
		c := n.Children[0]
		vert := "┆"
		if b.onPath[c.Entry.ID] {
			vert = "┃"
		}
		took(b.walk(c, lane, lane, lane, vert, false))
	} else if len(n.Children) > 1 {
		active := -1
		for i, c := range n.Children {
			if b.onPath[c.Entry.ID] {
				active = i
			}
		}
		for i, c := range n.Children {
			last := i == len(n.Children)-1
			var curve string
			switch {
			case active == i:
				curve = "┗━"
			case last:
				curve = "╰╌"
			case active > i:
				curve = "┃╌"
			default:
				curve = "├╌"
			}

			aboveV := "┆"
			if active >= i {
				aboveV = "┃"
			}
			childLane := lane + "  "
			if !last {
				childLane = lane + "┆ "
				if active > i {
					childLane = lane + "┃ "
				}
			}
			took(b.walk(c, childLane, lane+curve, lane, aboveV, false))
		}
	}

	b.stops[at].beyond, b.stops[at].latest = beyond, latest
	return beyond, latest
}

// The landmarks passed between a turn and the one before it: a compaction, a
// cleared context, a change of model. Scenery on the road, drawn where it was.
func (b *roads) landmarks(e session.Entry) []string {
	var marks []string
	seen := map[string]bool{}
	for at, ok := b.byID[e.Parent]; ok && !seen[at.ID]; at, ok = b.byID[at.Parent] {
		seen[at.ID] = true
		if session.IsTurn(at) {
			break
		}
		switch at.Type {
		case session.KindCompact:
			marks = append(marks, "≋ folded into a summary")
		case session.KindClear:
			marks = append(marks, "∅ context cleared")
		case session.KindModel:
			marks = append(marks, "⇄ model → "+at.Model)
		}
	}
	// Walking up reads them newest first; the road runs the other way.
	for i, j := 0, len(marks)-1; i < j; i, j = i+1, j-1 {
		marks[i], marks[j] = marks[j], marks[i]
	}
	return marks
}

func (m *Model) closeJourney() {
	m.journey = journey{}
	m.syncViewport()
}

func (m *Model) handleJourneyKey(msg tea.KeyMsg) tea.Cmd {
	switch key := msg.String(); key {
	case "up", "k", "ctrl+p", "shift+tab":
		m.moveJourney(-1)
	case "down", "j", "ctrl+n", "tab":
		m.moveJourney(1)
	case "pgup", "ctrl+u", "ctrl+b":
		m.moveJourney(-5)
	case "pgdown", "ctrl+d", "ctrl+f":
		m.moveJourney(5)
	case "home", "g":
		m.moveJourney(-len(m.journey.stops))
	case "end", "G":
		m.moveJourney(len(m.journey.stops))
	case "enter", "l", "o":
		if m.journey.cursor < len(m.journey.stops) {
			stop := m.journey.stops[m.journey.cursor]
			m.closeJourney()
			m.visit(stop)
		}
	case "b":
		if m.journey.cursor < len(m.journey.stops) {
			stop := m.journey.stops[m.journey.cursor]
			m.closeJourney()
			m.diverge(stop)
		}
	case "esc", "q", "h", "ctrl+c":
		m.closeJourney()
	}
	return nil
}

func (m *Model) moveJourney(delta int) {
	n := len(m.journey.stops)
	if n == 0 {
		return
	}
	m.journey.cursor = min(max(m.journey.cursor+delta, 0), n-1)
	m.aimJourneyBar()
	m.syncViewport()
}

// visit lands on a stop as it stood when its turn finished; diverge re-asks.
func (m *Model) visit(stop journeyStop) {
	if stop.here {
		m.notice("already here")
		return
	}
	store := m.rec.Store()
	if store == nil {
		return
	}
	entries, err := store.Entries()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}
	leaf, err := store.Leaf()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}

	tip := exchangeTip(entries, stop.id, pathSet(entries, leaf))
	ctx := session.BuildAt(entries, tip)
	if err := m.rec.Jump(tip, len(ctx.Messages)); err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}

	m.applyContext(ctx)
	m.push(entry{kind: entryNotice, text: fmt.Sprintf(
		"travelled · the conversation stands as it did %s, %d messages in", when(stop.at), len(ctx.Messages))})
}

// contextAt is what the provider counted for the newest turn at that point.
func contextAt(entries []session.Entry, leaf string) int {
	path := session.PathTo(entries, leaf)
	for i := len(path) - 1; i >= 0; i-- {
		switch e := path[i]; {
		case e.Type == session.KindClear || e.Type == session.KindCompact:
			return 0
		case e.Type == session.KindMessage && e.Usage != nil:
			u := *e.Usage
			return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens +
				u.OutputTokens + u.ThinkingTokens
		}
	}
	return 0
}

func pathSet(entries []session.Entry, leaf string) map[string]bool {
	onPath := map[string]bool{}
	for _, e := range session.PathTo(entries, leaf) {
		onPath[e.ID] = true
	}
	return onPath
}

// exchangeTip walks a prompt down to the last message of its own exchange.
func exchangeTip(entries []session.Entry, stop string, onPath map[string]bool) string {
	children := make(map[string][]session.Entry, len(entries))
	for _, e := range entries {
		children[e.Parent] = append(children[e.Parent], e)
	}

	at := stop
	for {
		next := ""
		for _, c := range children[at] {
			if c.Type != session.KindMessage || session.IsTurn(c) {
				continue
			}
			if next == "" || onPath[c.ID] {
				next = c.ID
			}
		}
		if next == "" {
			return at
		}
		at = next
	}
}

// diverge takes the conversation up again from any stop on the map: the
// context becomes everything that led there, and the prompt comes back to
// the composer to be changed and asked again. Rewind, but across roads.
func (m *Model) diverge(stop journeyStop) {
	store := m.rec.Store()
	if store == nil {
		return
	}
	entries, err := store.Entries()
	if err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}

	ctx := session.BuildAt(entries, stop.parent)
	if err := m.rec.Jump(stop.parent, len(ctx.Messages)); err != nil {
		m.push(entry{kind: entryError, text: err.Error()})
		return
	}

	m.applyContext(ctx)
	m.showRecalled(stop.prompt)
	m.push(entry{kind: entryNotice, text: fmt.Sprintf(
		"branched · %d messages lead here, and the prompt is back in the composer", len(ctx.Messages))})
}

// Two lines of heading, three of signpost.
const journeyChrome = 5

func (m *Model) journeyRowCount() int {
	return max(m.viewport.Height-journeyChrome, 3)
}

func (m *Model) journeyView() string {
	rows := m.journeyRowCount()
	window, _ := m.journeyWindow(rows)

	lines := make([]string, 0, rows+journeyChrome)
	lines = append(lines, m.journeyHeading())
	lines = append(lines, "")

	for i, line := range window {
		marker := m.journeyBar(i)
		switch line.kind {
		case lineStop:
			lines = append(lines, marker+" "+m.stopRow(line))
		case lineLandmark:
			lines = append(lines, marker+" "+paveRoad(line.road)+metaStyle.Render(line.text))
		default:
			lines = append(lines, strings.TrimRight(marker+" "+paveRoad(line.road), " "))
		}
	}

	if pad := m.viewport.Height - len(lines) - 3; pad > 0 {
		lines = append(lines, make([]string, pad)...)
	}
	return strings.Join(append(lines, m.journeySignpost()...), "\n")
}

func (m *Model) stopRow(line journeyLine) string {
	stop := m.journey.stops[line.stop]

	mark, markStyle, style := "●", userMarker, bodyStyle
	if !stop.onPath {
		mark, markStyle, style = "○", metaStyle, metaStyle
	}
	if stop.here {
		mark = "◉"
	}
	if line.stop == m.journey.cursor {
		style = menuPickStyle
	}

	left := paveRoad(line.road) + markStyle.Render(mark) + " "
	facts := when(stop.at)
	if stop.tokens > 0 {
		facts += " · " + humanTokens(stop.tokens)
	}
	ago := metaStyle.Render(facts)
	if stop.here {
		ago = userMarker.Render("you are here") + metaStyle.Render(" · "+facts)
	}
	room := max(m.contentWidth()-lipgloss.Width(left)-lipgloss.Width(ago)-4, 12)

	// The age sits in a column at the right edge, so the raggedness of the
	// prompts does not carry into it.
	row := left + style.Render(truncate(firstLine(stop.prompt), room))
	gap := max(m.contentWidth()-lipgloss.Width(row)-lipgloss.Width(ago)-2, 2)
	return row + strings.Repeat(" ", gap) + ago
}

// The taken road is drawn heavy and warm; everything left behind is thin,
// dashed, and the colour of the ground.
func paveRoad(road string) string {
	var b strings.Builder
	for _, r := range road {
		switch r {
		case '┃', '━', '┗':
			b.WriteString(packStyle.Render(string(r)))
		case ' ':
			b.WriteRune(r)
		default:
			b.WriteString(gutterStyle.Render(string(r)))
		}
	}
	return b.String()
}

// What the highlighted stop's signpost says: the prompt whole, and what
// taking this road again would mean.
func (m *Model) journeySignpost() []string {
	if m.journey.cursor >= len(m.journey.stops) {
		return []string{"", "", ""}
	}
	stop := m.journey.stops[m.journey.cursor]

	facts := fmt.Sprintf("⏎ lands here, asked and answered, %d messages in · b re-asks it", stop.messages)
	if stop.beyond == 1 {
		facts += " · one turn lies beyond"
	} else if stop.beyond > 1 {
		facts += fmt.Sprintf(" · %d turns lie beyond", stop.beyond)
	}
	facts += " · last travelled " + when(stop.latest)

	return []string{
		gutterStyle.Render(strings.Repeat("╌", max(m.contentWidth()-1, 10))),
		userMarker.Render("›") + " " +
			bodyStyle.Render(truncate(firstLine(stop.prompt), m.contentWidth()-4)),
		"  " + hintStyle.Render(clamp(facts, m.contentWidth()-2)),
	}
}

func (m *Model) journeyWindow(rows int) ([]journeyLine, int) {
	at := m.journey.stops[m.journey.cursor].line
	if len(m.journey.lines) <= rows {
		return m.journey.lines, at
	}
	start := min(max(at-rows/2, 0), len(m.journey.lines)-rows)
	return m.journey.lines[start : start+rows], at - start
}

func (m *Model) journeyHeading() string {
	turns := fmt.Sprintf("journey · turn %d of %d", m.journey.cursor+1, len(m.journey.stops))
	if m.journey.branches <= 1 {
		return metaStyle.Render(turns + " · one road so far")
	}
	return metaStyle.Render(fmt.Sprintf("%s · %d roads", turns, m.journey.branches))
}

func (m *Model) journeyFooter() string {
	return hintStyle.Render("j/k or ↑↓ move · ⏎ go there · b branch & re-ask · g/G ends · q back")
}

// The journey's bar is the picker's, and it travels the roads between stops
// rather than jumping them.
func (m *Model) journeyBar(row int) string {
	if row == m.journey.barAt {
		return cursorBar()
	}
	if life, ok := m.journey.trail[row]; ok {
		return trailBar((life + framesPerShade - 1) / framesPerShade)
	}
	return noBar()
}

func (m *Model) aimJourneyBar() tea.Cmd {
	_, cursor := m.journeyWindow(m.journeyRowCount())
	m.journey.barTo = cursor
	if !m.motion.on {
		m.journey.barAt = cursor
		return nil
	}
	return m.animate()
}

func (m *Model) advanceJourneyBar() bool {
	if !m.journey.open || m.journey.barAt == m.journey.barTo {
		return m.fadeJourneyTrail()
	}

	m.leaveJourneyTrail(m.journey.barAt)
	if m.journey.barAt < m.journey.barTo {
		m.journey.barAt++
	} else {
		m.journey.barAt--
	}
	return true
}

func (m *Model) leaveJourneyTrail(row int) {
	if m.journey.trail == nil {
		m.journey.trail = map[int]int{}
	}
	m.journey.trail[row] = trailLife()
}

func (m *Model) fadeJourneyTrail() bool {
	if len(m.journey.trail) == 0 {
		return false
	}
	for row, life := range m.journey.trail {
		if life <= 1 {
			delete(m.journey.trail, row)
			continue
		}
		m.journey.trail[row] = life - 1
	}
	return true
}
