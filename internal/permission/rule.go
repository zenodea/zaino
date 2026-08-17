package permission

import (
	"fmt"
	"strings"
)

// A Rule is a standing answer, so that a thing you would always say yes to
// stops asking: "bash" for the whole tool, "bash:git status" for the commands
// starting that way, "write:internal/" for a corner of the tree. "*" is any
// tool, and a trailing star is decoration — the target always matches by its
// start, since that is the part a rule can be sure of.
type Rule struct {
	Tool   string
	Prefix string
}

func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, fmt.Errorf("empty rule")
	}

	name, prefix, _ := strings.Cut(s, ":")
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return Rule{}, fmt.Errorf("%q names no tool — write tool or tool:start-of-target", s)
	}
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "*")
	return Rule{Tool: name, Prefix: prefix}, nil
}

func ParseRules(list []string) ([]Rule, error) {
	out := make([]Rule, 0, len(list))
	for _, s := range list {
		rule, err := ParseRule(s)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

func (r Rule) Matches(req Request) bool {
	if r.Tool != "*" && !strings.EqualFold(r.Tool, req.Tool) {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(req.Target), r.Prefix)
}

func (r Rule) String() string {
	if r.Prefix == "" {
		return r.Tool
	}
	return r.Tool + ":" + r.Prefix
}

func matches(rules []Rule, req Request) bool {
	for _, rule := range rules {
		if rule.Matches(req) {
			return true
		}
	}
	return false
}
