package recall

import (
	"bufio"
	"os"
	"strings"

	"github.com/zenodea/zaino/internal/x/fsx"
	"github.com/zenodea/zaino/internal/x/paths"
)

const DefaultLimit = 500

type List struct {
	path  string
	limit int
	lines []string

	at    int
	draft string
}

func Open() (*List, error) {
	path, err := paths.Data("recall")
	if err != nil {
		return New(), err
	}
	return Load(path, DefaultLimit)
}

func New() *List {
	return &List{limit: DefaultLimit, at: -1}
}

func Load(path string, limit int) (*List, error) {
	l := &List{path: path, limit: limit, at: -1}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return l, err
	}
	defer f.Close()

	var lines []string
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scan.Scan() {
		if line := unescape(scan.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	if err := scan.Err(); err != nil {
		return l, err
	}

	trimmed := len(lines) > limit
	if trimmed {
		lines = lines[len(lines)-limit:]
	}
	l.lines = make([]string, len(lines))
	for i, line := range lines {
		l.lines[len(lines)-1-i] = line
	}

	if trimmed {
		return l, l.rewrite()
	}
	return l, nil
}

func (l *List) Lines() []string { return l.lines }

func (l *List) Add(line string) error {
	l.Reset()

	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if len(l.lines) > 0 && l.lines[0] == line {
		return nil
	}

	l.lines = append([]string{line}, l.lines...)
	if l.limit > 0 && len(l.lines) > l.limit {
		l.lines = l.lines[:l.limit]
	}
	if l.path == "" {
		return nil
	}
	return fsx.AppendLine(l.path, escape(line))
}

func (l *List) Browsing() bool { return l.at >= 0 }

func (l *List) Prev(draft string) (string, bool) {
	if l.at+1 >= len(l.lines) {
		return "", false
	}
	if l.at < 0 {
		l.draft = draft
	}
	l.at++
	return l.lines[l.at], true
}

func (l *List) Next() (string, bool) {
	switch {
	case l.at < 0:
		return "", false
	case l.at == 0:
		l.at = -1
		draft := l.draft
		l.draft = ""
		return draft, true
	default:
		l.at--
		return l.lines[l.at], true
	}
}

func (l *List) Reset() {
	l.at = -1
	l.draft = ""
}

func (l *List) rewrite() error {
	if l.path == "" {
		return nil
	}
	var b strings.Builder
	for i := len(l.lines) - 1; i >= 0; i-- {
		b.WriteString(escape(l.lines[i]))
		b.WriteByte('\n')
	}
	return fsx.WriteAtomic(l.path, []byte(b.String()), 0o600)
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return strings.ReplaceAll(s, "\r", `\r`)
}

func unescape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
