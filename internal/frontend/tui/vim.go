package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
)

type vimMode int

const (
	modeInsert vimMode = iota
	modeNormal
)

func (v vimMode) String() string {
	if v == modeNormal {
		return "NORMAL"
	}
	return "INSERT"
}

func (m *Model) modeName() string {
	switch {
	case m.inVisualMode() && m.vim.lineWise:
		return "V-LINE"
	case m.inVisualMode():
		return "VISUAL"
	}
	return m.vim.mode.String()
}

type vim struct {
	on   bool
	mode vimMode

	operator rune
	count    int
	register string

	visual   bool
	lineWise bool
	anchor   int

	undo []undoStep
}

type undoStep struct {
	text string
	pos  int
}

func (m *Model) UseVim(on bool) {
	m.vim.on = on
	m.enterInsert()
}

func (m *Model) vimEnabled() bool { return m.vim.on }

func (m *Model) inNormalMode() bool { return m.vim.on && m.vim.mode == modeNormal }

func (m *Model) enterNormal() {
	m.vim.mode = modeNormal
	m.vim.operator, m.vim.count = 0, 0
	m.input.Cursor.SetMode(cursor.CursorStatic)

	text, pos := m.buffer()
	if pos > 0 && (pos >= len(text) || text[pos] == '\n') && pos-1 >= 0 && text[pos-1] != '\n' {
		m.setBuffer(text, pos-1)
	}
}

func (m *Model) enterInsert() {
	m.vim.mode = modeInsert
	m.vim.visual = false
	m.vim.operator, m.vim.count = 0, 0
	m.input.Cursor.SetMode(cursor.CursorBlink)
}

func (m *Model) buffer() ([]rune, int) {
	text := []rune(m.input.Value())

	line, col := m.input.Line(), m.cursorColumn()
	pos, seen := len(text), 0
	for i, r := range text {
		if seen == line {
			pos = min(i+col, lineEnd(text, i))
			break
		}
		if r == '\n' {
			seen++
		}
	}
	if line == 0 {
		pos = min(col, lineEnd(text, 0))
	}
	return text, min(max(pos, 0), len(text))
}

func (m *Model) cursorColumn() int {
	info := m.input.LineInfo()
	return info.StartColumn + info.ColumnOffset
}

func lineEnd(text []rune, from int) int {
	for i := from; i < len(text); i++ {
		if text[i] == '\n' {
			return i
		}
	}
	return len(text)
}

func (m *Model) setBuffer(text []rune, pos int) {
	pos = min(max(pos, 0), len(text))
	m.input.SetValue(string(text))

	line, col := 0, 0
	for i := range pos {
		if text[i] == '\n' {
			line, col = line+1, 0
			continue
		}
		col++
	}

	for m.input.Line() > line {
		m.input.CursorUp()
	}
	for m.input.Line() < line {
		m.input.CursorDown()
	}
	m.input.SetCursor(col)
	m.syncInputChrome()
}

func (m *Model) pushUndo() {
	text, pos := m.buffer()
	m.vim.undo = append(m.vim.undo, undoStep{text: string(text), pos: pos})
	if len(m.vim.undo) > 100 {
		m.vim.undo = m.vim.undo[1:]
	}
}

func (m *Model) popUndo() bool {
	if len(m.vim.undo) == 0 {
		return false
	}
	step := m.vim.undo[len(m.vim.undo)-1]
	m.vim.undo = m.vim.undo[:len(m.vim.undo)-1]
	m.setBuffer([]rune(step.text), step.pos)
	return true
}

func (m *Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key != "ctrl+c" {
		m.quitArmed = false
	}

	switch key {
	case "enter":
		if m.toggleSelected() {
			return m, nil
		}
		return m, m.submit()
	case "ctrl+c", "shift+tab", "ctrl+j", "ctrl+k":
		return m.handleAppKey(msg)
	case "ctrl+u", "ctrl+d", "ctrl+f", "ctrl+b", "pgup", "pgdown":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' || (key == "0" && m.vim.count > 0) {
		m.vim.count = m.vim.count*10 + int(key[0]-'0')
		return m, nil
	}

	count := max(m.vim.count, 1)
	text, pos := m.buffer()

	if m.vim.operator != 0 {
		return m, m.applyOperator(key, text, pos, count)
	}

	if m.vim.visual {
		if model, cmd, handled := m.handleVisualKey(msg); handled {
			m.vim.count = 0
			return model, cmd
		}
	}

	switch key {
	case "esc":
		// Nothing left to cancel in the editor, so esc means the turn.
		if m.vim.count == 0 && m.vim.operator == 0 && m.streaming && m.cancel != nil {
			m.cancel()
		}
		m.vim.count, m.vim.operator = 0, 0
		return m, nil

	case "v":
		m.enterVisual()
		return m, nil

	case "V":
		m.enterVisualLine()
		return m, nil

	case "i":
		m.enterInsert()
	case "a":
		m.enterInsert()
		m.setBuffer(text, min(pos+1, lineEnd(text, pos)))
	case "I":
		m.enterInsert()
		m.setBuffer(text, firstNonBlank(text, pos))
	case "A":
		m.enterInsert()
		m.setBuffer(text, lineEnd(text, pos))
	case "o":
		m.pushUndo()
		m.enterInsert()
		at := lineEnd(text, pos)
		m.setBuffer(insert(text, at, []rune{'\n'}), at+1)
	case "O":
		m.pushUndo()
		m.enterInsert()
		at := lineStart(text, pos)
		m.setBuffer(insert(text, at, []rune{'\n'}), at)

	case "h":
		m.setBuffer(text, backward(text, pos, count))
	case "l":
		m.setBuffer(text, forward(text, pos, count))
	case "j", "k":
		if m.cursor >= 0 && strings.TrimSpace(string(text)) == "" {
			return m.handleAppKey(msg)
		}
		return m, m.verticalMove(key, count, text, pos)
	case "w":
		m.setBuffer(text, wordForward(text, pos, count))
	case "b":
		m.setBuffer(text, wordBackward(text, pos, count))
	case "e":
		m.setBuffer(text, wordEnd(text, pos, count))
	case "0":
		m.setBuffer(text, lineStart(text, pos))
	case "^":
		m.setBuffer(text, firstNonBlank(text, pos))
	case "$":
		m.setBuffer(text, max(lineEnd(text, pos)-1, lineStart(text, pos)))
	case "G":
		m.setBuffer(text, len(text))
	case "g":
		m.vim.operator = 'g'
		return m, nil

	case "x":
		m.pushUndo()
		end := min(pos+count, lineEnd(text, pos))
		m.vim.register = string(text[pos:end])
		m.setBuffer(cut(text, pos, end), pos)
	case "X":
		m.pushUndo()
		start := max(pos-count, lineStart(text, pos))
		m.setBuffer(cut(text, start, pos), start)
	case "D":
		m.pushUndo()
		m.vim.register = string(text[pos:lineEnd(text, pos)])
		m.setBuffer(cut(text, pos, lineEnd(text, pos)), pos)
	case "C":
		m.pushUndo()
		m.enterInsert()
		m.vim.register = string(text[pos:lineEnd(text, pos)])
		m.setBuffer(cut(text, pos, lineEnd(text, pos)), pos)
	case "d", "c":
		m.vim.operator = rune(key[0])
		return m, nil

	case "p":
		m.pushUndo()
		at := min(pos+1, len(text))
		m.setBuffer(insert(text, at, []rune(m.vim.register)), at+len([]rune(m.vim.register))-1)
	case "P":
		m.pushUndo()
		m.setBuffer(insert(text, pos, []rune(m.vim.register)), pos+max(len([]rune(m.vim.register))-1, 0))

	case "u":
		if !m.popUndo() {
			m.notice("nothing to undo")
		}

	case "up":
		if m.recallPrev() {
			return m, nil
		}
	case "down":
		if m.recallNext() {
			return m, nil
		}
	}

	m.vim.count = 0
	return m, nil
}

func (m *Model) verticalMove(key string, count int, text []rune, pos int) tea.Cmd {
	if strings.Count(string(text), "\n") == 0 {
		delta := count
		if key == "k" {
			delta = -count
		}
		m.viewport.SetYOffset(m.viewport.YOffset + delta)
		return nil
	}

	for range count {
		if key == "j" {
			m.input.CursorDown()
			continue
		}
		m.input.CursorUp()
	}
	_ = pos
	return nil
}

func (m *Model) applyOperator(key string, text []rune, pos int, count int) tea.Cmd {
	operator := m.vim.operator
	m.vim.operator, m.vim.count = 0, 0

	if operator == 'g' {
		if key == "g" {
			m.setBuffer(text, 0)
		}
		return nil
	}

	var from, to int
	switch key {
	case "d", "c":
		if rune(key[0]) != operator {
			return nil
		}
		from, to = lineStart(text, pos), lineEnd(text, pos)
		if operator == 'd' && to < len(text) {
			to++
		}
	case "w":
		from, to = pos, wordForward(text, pos, count)
	case "e":
		from, to = pos, min(wordEnd(text, pos, count)+1, len(text))
	case "b":
		from, to = wordBackward(text, pos, count), pos
	case "$":
		from, to = pos, lineEnd(text, pos)
	case "0":
		from, to = lineStart(text, pos), pos
	case "^":
		from, to = firstNonBlank(text, pos), pos
	default:
		return nil
	}

	if from >= to {
		return nil
	}
	m.pushUndo()
	m.vim.register = string(text[from:to])
	if operator == 'c' {
		m.enterInsert()
	}
	m.setBuffer(cut(text, from, to), from)
	return nil
}

func insert(text []rune, at int, what []rune) []rune {
	out := make([]rune, 0, len(text)+len(what))
	out = append(out, text[:at]...)
	out = append(out, what...)
	return append(out, text[at:]...)
}

func cut(text []rune, from, to int) []rune {
	out := make([]rune, 0, len(text)-(to-from))
	out = append(out, text[:from]...)
	return append(out, text[to:]...)
}

func lineStart(text []rune, pos int) int {
	for i := min(pos, len(text)) - 1; i >= 0; i-- {
		if text[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

func firstNonBlank(text []rune, pos int) int {
	at := lineStart(text, pos)
	for at < len(text) && text[at] != '\n' && unicode.IsSpace(text[at]) {
		at++
	}
	return at
}

func forward(text []rune, pos, count int) int {
	return min(pos+count, max(lineEnd(text, pos)-1, lineStart(text, pos)))
}

func backward(text []rune, pos, count int) int {
	return max(pos-count, lineStart(text, pos))
}

func classOf(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	default:
		return 2
	}
}

func wordForward(text []rune, pos, count int) int {
	for range count {
		if pos >= len(text) {
			break
		}
		start := classOf(text[pos])
		for pos < len(text) && classOf(text[pos]) == start && start != 0 {
			pos++
		}
		for pos < len(text) && classOf(text[pos]) == 0 {
			pos++
		}
	}
	return min(pos, len(text))
}

func wordBackward(text []rune, pos, count int) int {
	for range count {
		pos--
		for pos > 0 && classOf(text[pos]) == 0 {
			pos--
		}
		if pos <= 0 {
			return 0
		}
		class := classOf(text[pos])
		for pos > 0 && classOf(text[pos-1]) == class {
			pos--
		}
	}
	return max(pos, 0)
}

func wordEnd(text []rune, pos, count int) int {
	for range count {
		pos++
		for pos < len(text) && classOf(text[pos]) == 0 {
			pos++
		}
		if pos >= len(text) {
			return len(text) - 1
		}
		class := classOf(text[pos])
		for pos+1 < len(text) && classOf(text[pos+1]) == class {
			pos++
		}
	}
	return min(pos, max(len(text)-1, 0))
}
