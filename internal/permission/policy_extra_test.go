package permission

import (
	"slices"
	"sync"
	"testing"
)

func TestModeDescribeCoversEveryMode(t *testing.T) {
	for _, name := range ModeNames() {
		mode := Mode(name)
		desc := mode.Describe()
		if desc == "" || desc == name {
			t.Errorf("%s has no description of its own: %q", name, desc)
		}
	}
	if got := Mode("invented").Describe(); got != "invented" {
		t.Errorf("an unknown mode should describe itself, got %q", got)
	}
}

func TestNewPolicyDefaultsToAsking(t *testing.T) {
	if got := NewPolicy("").Mode; got != Manual {
		t.Errorf("got %q, want manual", got)
	}
}

func TestSetModeChangesTheVerdict(t *testing.T) {
	p := NewPolicy(Manual)
	req := Request{Tool: "write", Action: Write, Target: "x.go"}

	if v, _ := p.Decide(req); v != Ask {
		t.Fatalf("got %v, want Ask", v)
	}
	p.SetMode(Bypass)
	if v, _ := p.Decide(req); v != Allow {
		t.Errorf("got %v, want Allow after bypass", v)
	}
	p.SetMode(Plan)
	if v, _ := p.Decide(req); v != Deny {
		t.Errorf("got %v, want Deny in plan mode", v)
	}
}

func TestGrantedIsEmptyBeforeAnythingIsRemembered(t *testing.T) {
	if got := NewPolicy(Manual).Granted(); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestGrantedReadsBackAsToolAndTarget(t *testing.T) {
	p := NewPolicy(Manual)
	p.Remember(Request{Tool: "bash", Action: Execute, Target: "git status --short"})
	p.Remember(Request{Tool: "write", Action: Write, Target: "main.go"})

	got := p.Granted()
	if !slices.Contains(got, "bash git") {
		t.Errorf("got %v, want it to contain %q", got, "bash git")
	}
	if !slices.Contains(got, "write main.go") {
		t.Errorf("got %v, want it to contain %q", got, "write main.go")
	}
}

func TestRememberOnAZeroPolicyDoesNotPanic(t *testing.T) {
	var p Policy
	p.Remember(Request{Tool: "write", Action: Write, Target: "x.go"})
	if got := p.Granted(); len(got) != 1 {
		t.Errorf("got %v, want one grant", got)
	}
}

func TestPolicyIsSafeUnderConcurrentUse(t *testing.T) {
	p := NewPolicy(Manual)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			p.Remember(Request{Tool: "bash", Action: Execute, Target: string(rune('a' + i%26))})
		}()
		go func() { defer wg.Done(); p.Decide(Request{Tool: "write", Action: Write, Target: "x"}) }()
		go func() { defer wg.Done(); p.Granted() }()
	}
	wg.Wait()
}

func TestGateModeWithoutAPolicyIsBypass(t *testing.T) {
	var g *Gate
	if got := g.Mode(); got != Bypass {
		t.Errorf("nil gate: got %q, want bypass", got)
	}
	if got := (&Gate{}).Mode(); got != Bypass {
		t.Errorf("policyless gate: got %q, want bypass", got)
	}
}

func TestGateModeReportsThePolicy(t *testing.T) {
	g := &Gate{Policy: NewPolicy(Plan)}
	if got := g.Mode(); got != Plan {
		t.Errorf("got %q, want plan", got)
	}
	g.Policy.SetMode(AcceptEdits)
	if got := g.Mode(); got != AcceptEdits {
		t.Errorf("got %q, want accept-edits", got)
	}
}

func TestDeniedErrorNamesTheToolAndReason(t *testing.T) {
	err := &DeniedError{Request: Request{Tool: "bash"}, Reason: "you said no"}
	if got, want := err.Error(), "bash denied: you said no"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !Denied(err) {
		t.Error("Denied() = false for a DeniedError")
	}
}
