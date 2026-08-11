package permission

import (
	"strings"
	"sync"
)

type Verdict int

const (
	Ask Verdict = iota
	Allow
	Deny
)

type Policy struct {
	Mode Mode

	AllowOutside bool

	mu      sync.Mutex
	granted map[string]bool
}

func NewPolicy(mode Mode) *Policy {
	if mode == "" {
		mode = Manual
	}
	return &Policy{Mode: mode, granted: map[string]bool{}}
}

func (p *Policy) Decide(req Request) (Verdict, string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Mode == Bypass {
		return Allow, ""
	}
	if req.Outside && !p.AllowOutside {
		return Deny, "outside the workspace — rerun with -allow-outside to permit this"
	}
	if req.Action == Read {
		return Allow, ""
	}
	if p.Mode == Plan {
		return Deny, "plan mode is read only"
	}
	if p.granted[grantKey(req)] {
		return Allow, ""
	}
	if p.Mode == AcceptEdits && req.Action == Write {
		return Allow, ""
	}
	return Ask, ""
}

func (p *Policy) Remember(req Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.granted == nil {
		p.granted = map[string]bool{}
	}
	p.granted[grantKey(req)] = true
}

func (p *Policy) SetMode(m Mode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Mode = m
}

func (p *Policy) Granted() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.granted))
	for k := range p.granted {
		out = append(out, strings.ReplaceAll(k, "\x00", " "))
	}
	return out
}

// A remembered "always" for a command must not hand over the whole shell, so
// it matches the program only: yes to `git`, not to everything after it.
func grantKey(req Request) string {
	target := req.Target
	if req.Action == Execute {
		target, _, _ = strings.Cut(strings.TrimSpace(target), " ")
	}
	return req.Tool + "\x00" + target
}
