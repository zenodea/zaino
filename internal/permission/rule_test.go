package permission

import "testing"

func TestParseRule(t *testing.T) {
	tests := []struct {
		in     string
		tool   string
		prefix string
	}{
		{"bash", "bash", ""},
		{"bash:git status", "bash", "git status"},
		{"bash: go test *", "bash", "go test "},
		{"WRITE:internal/", "write", "internal/"},
		{"*", "*", ""},
	}

	for _, tt := range tests {
		got, err := ParseRule(tt.in)
		if err != nil {
			t.Fatalf("ParseRule(%q): %v", tt.in, err)
		}
		if got.Tool != tt.tool || got.Prefix != tt.prefix {
			t.Errorf("ParseRule(%q) = %+v, want %s/%s", tt.in, got, tt.tool, tt.prefix)
		}
	}

	for _, bad := range []string{"", "   ", ":no tool"} {
		if _, err := ParseRule(bad); err == nil {
			t.Errorf("ParseRule(%q) = nil error, want one", bad)
		}
	}
}

func TestRuleMatches(t *testing.T) {
	req := Request{Tool: "bash", Action: Execute, Target: "git status --short"}

	tests := []struct {
		rule string
		want bool
	}{
		{"bash", true},
		{"bash:git", true},
		{"bash:git status", true},
		{"bash:git push", false},
		{"read", false},
		{"*:git", true},
	}

	for _, tt := range tests {
		rule, err := ParseRule(tt.rule)
		if err != nil {
			t.Fatal(err)
		}
		if got := rule.Matches(req); got != tt.want {
			t.Errorf("%q matched %v, want %v", tt.rule, got, tt.want)
		}
	}
}

func TestAllowRuleAnswersForYou(t *testing.T) {
	p := NewPolicy(Manual)
	req := Request{Tool: "bash", Action: Execute, Target: "go test ./..."}

	if verdict, _ := p.Decide(req); verdict != Ask {
		t.Fatalf("verdict = %v, want Ask before any rule", verdict)
	}

	rules, err := ParseRules([]string{"bash:go test"})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRules(rules, nil)

	if verdict, _ := p.Decide(req); verdict != Allow {
		t.Errorf("verdict = %v, want Allow", verdict)
	}
	if verdict, _ := p.Decide(Request{Tool: "bash", Action: Execute, Target: "go build"}); verdict != Ask {
		t.Error("the rule reached further than its prefix")
	}
}

// A deny rule is written down in advance, so it holds where zaino would not
// otherwise have asked at all: reading, and bypass mode.
func TestDenyRuleBeatsTheQuietPaths(t *testing.T) {
	rules, err := ParseRules([]string{"read:secrets/"})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Tool: "read", Action: Read, Target: "secrets/keys.json"}

	for _, mode := range []Mode{Manual, AcceptEdits, Bypass} {
		p := NewPolicy(mode)
		p.SetRules(nil, rules)
		if verdict, reason := p.Decide(req); verdict != Deny {
			t.Errorf("%s: verdict = %v (%s), want Deny", mode, verdict, reason)
		}
	}
}

func TestRulesReadBack(t *testing.T) {
	p := NewPolicy(Manual)
	allow, err := ParseRules([]string{"bash:git status"})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRules(allow, nil)

	got, _ := p.Rules()
	if len(got) != 1 || got[0].String() != "bash:git status" {
		t.Errorf("rules = %v", got)
	}
}
