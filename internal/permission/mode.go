package permission

import (
	"fmt"
	"strings"
)

type Mode string

const (
	Manual      Mode = "manual"
	AcceptEdits Mode = "accept-edits"
	Plan        Mode = "plan"
	Bypass      Mode = "bypass"
)

var modes = []Mode{Manual, AcceptEdits, Plan, Bypass}

var aliases = map[string]Mode{
	"manual":       Manual,
	"default":      Manual,
	"ask":          Manual,
	"accept-edits": AcceptEdits,
	"acceptedits":  AcceptEdits,
	"edits":        AcceptEdits,
	"plan":         Plan,
	"read-only":    Plan,
	"bypass":       Bypass,
	"yolo":         Bypass,
}

func ParseMode(s string) (Mode, error) {
	if m, ok := aliases[strings.ToLower(strings.TrimSpace(s))]; ok {
		return m, nil
	}
	return "", fmt.Errorf("unknown permission mode %q — pick one of %s", s, strings.Join(ModeNames(), ", "))
}

func ModeNames() []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = string(m)
	}
	return out
}

func (m Mode) Describe() string {
	switch m {
	case Manual:
		return "ask before writing or running anything"
	case AcceptEdits:
		return "edits go through, ask before running anything"
	case Plan:
		return "read only, nothing is written or run"
	case Bypass:
		return "everything goes through unasked"
	}
	return string(m)
}

func (m Mode) Next() Mode {
	for i, at := range modes {
		if at == m {
			return modes[(i+1)%len(modes)]
		}
	}
	return Manual
}
