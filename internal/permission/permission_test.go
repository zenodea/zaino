package permission

import (
	"context"
	"errors"
	"testing"
)

func read() Request    { return Request{Tool: "read", Action: Read, Target: "main.go"} }
func write() Request   { return Request{Tool: "edit", Action: Write, Target: "main.go"} }
func execute() Request { return Request{Tool: "bash", Action: Execute, Target: "git status"} }

func TestDecide(t *testing.T) {
	outside := Request{Tool: "read", Action: Read, Target: "/etc/hosts", Outside: true}

	tests := []struct {
		name string
		mode Mode
		req  Request
		want Verdict
	}{
		{"manual reads", Manual, read(), Allow},
		{"manual writes", Manual, write(), Ask},
		{"manual runs", Manual, execute(), Ask},

		{"accept-edits reads", AcceptEdits, read(), Allow},
		{"accept-edits writes", AcceptEdits, write(), Allow},
		{"accept-edits still asks to run", AcceptEdits, execute(), Ask},

		{"plan reads", Plan, read(), Allow},
		{"plan refuses writes", Plan, write(), Deny},
		{"plan refuses runs", Plan, execute(), Deny},

		{"bypass writes", Bypass, write(), Allow},
		{"bypass runs", Bypass, execute(), Allow},

		{"outside is refused, not asked", Manual, outside, Deny},
		{"outside is refused in accept-edits", AcceptEdits, outside, Deny},
		{"bypass reaches outside", Bypass, outside, Allow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := NewPolicy(tt.mode).Decide(tt.req)
			if got != tt.want {
				t.Errorf("Decide() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowOutside(t *testing.T) {
	p := NewPolicy(Manual)
	p.AllowOutside = true

	req := Request{Tool: "read", Action: Read, Target: "/etc/hosts", Outside: true}
	if got, _ := p.Decide(req); got != Allow {
		t.Errorf("Decide() = %v, want Allow once outside is permitted", got)
	}
}

func TestRememberedGrant(t *testing.T) {
	p := NewPolicy(Manual)
	if got, _ := p.Decide(write()); got != Ask {
		t.Fatalf("Decide() = %v, want Ask before remembering", got)
	}

	p.Remember(write())
	if got, _ := p.Decide(write()); got != Allow {
		t.Errorf("Decide() = %v, want Allow after remembering", got)
	}

	other := Request{Tool: "edit", Action: Write, Target: "other.go"}
	if got, _ := p.Decide(other); got != Ask {
		t.Errorf("Decide() = %v, want Ask — the grant was for one file", got)
	}
}

func TestRememberedCommandCoversProgramOnly(t *testing.T) {
	p := NewPolicy(Manual)
	p.Remember(execute())

	sameProgram := Request{Tool: "bash", Action: Execute, Target: "git log --oneline"}
	if got, _ := p.Decide(sameProgram); got != Allow {
		t.Errorf("Decide() = %v, want Allow for the same program", got)
	}

	other := Request{Tool: "bash", Action: Execute, Target: "rm -rf /"}
	if got, _ := p.Decide(other); got != Ask {
		t.Errorf("Decide() = %v, want Ask for a different program", got)
	}
}

type approver struct {
	grant Grant
	err   error
	asked int
}

func (a *approver) Approve(context.Context, Request) (Grant, error) {
	a.asked++
	return a.grant, a.err
}

func TestGate(t *testing.T) {
	tests := []struct {
		name      string
		grant     Grant
		wantErr   bool
		wantAsked int
	}{
		{"approved once", Once, false, 1},
		{"approved always", Always, false, 1},
		{"rejected", Reject, true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &approver{grant: tt.grant}
			g := &Gate{Policy: NewPolicy(Manual), Approver: a}

			err := g.Check(context.Background(), write())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && !Denied(err) {
				t.Errorf("Check() error = %v, want a DeniedError", err)
			}
			if a.asked != tt.wantAsked {
				t.Errorf("asked %d times, want %d", a.asked, tt.wantAsked)
			}
		})
	}
}

func TestGateAlwaysStopsAsking(t *testing.T) {
	a := &approver{grant: Always}
	g := &Gate{Policy: NewPolicy(Manual), Approver: a}

	for range 3 {
		if err := g.Check(context.Background(), write()); err != nil {
			t.Fatalf("Check() = %v, want allowed", err)
		}
	}
	if a.asked != 1 {
		t.Errorf("asked %d times, want 1 — the grant should have been remembered", a.asked)
	}
}

func TestGateWithoutApproverDenies(t *testing.T) {
	g := &Gate{Policy: NewPolicy(Manual)}
	if err := g.Check(context.Background(), write()); !Denied(err) {
		t.Errorf("Check() = %v, want denied when nothing can ask", err)
	}
}

func TestNilGateAllows(t *testing.T) {
	var g *Gate
	if err := g.Check(context.Background(), execute()); err != nil {
		t.Errorf("Check() = %v, want allowed", err)
	}
}

func TestApproverErrorIsNotADenial(t *testing.T) {
	fail := errors.New("interrupted")
	g := &Gate{Policy: NewPolicy(Manual), Approver: &approver{err: fail}}

	err := g.Check(context.Background(), write())
	if !errors.Is(err, fail) {
		t.Errorf("Check() = %v, want the approver's error to travel up", err)
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"manual", Manual, false},
		{"default", Manual, false},
		{"accept-edits", AcceptEdits, false},
		{"edits", AcceptEdits, false},
		{"Plan", Plan, false},
		{"yolo", Bypass, false},
		{" bypass ", Bypass, false},
		{"nonsense", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMode(%q) error = %v, want error %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestModeCycles(t *testing.T) {
	seen := map[Mode]bool{}
	m := Manual
	for range len(modes) {
		if seen[m] {
			t.Fatalf("Next() repeated %q before covering every mode", m)
		}
		seen[m] = true
		m = m.Next()
	}
	if m != Manual {
		t.Errorf("Next() ended on %q, want back at Manual", m)
	}
}
