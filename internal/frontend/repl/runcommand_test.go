package repl

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type stubProvider struct {
	name   string
	model  string
	models []string
}

func (s *stubProvider) Name() string         { return s.name }
func (s *stubProvider) DefaultModel() string { return s.model }
func (s *stubProvider) Models() []string     { return s.models }
func (s *stubProvider) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return nil, io.EOF
}

func newAgent() *agent.Agent {
	return &agent.Agent{
		Provider: &stubProvider{name: "stub", model: "stub-1", models: []string{"stub-1", "stub-2"}},
		Gate:     &permission.Gate{Policy: permission.NewPolicy(permission.Manual)},
	}
}

// runCommand narrates to stderr; the tests read that back to assert on it.
func run(t *testing.T, ag *agent.Agent, line string, o Options) (out string, cleared, quit bool) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	var usage llm.Usage
	did := runCommand(ag, line, nil, &usage, o)
	cleared, quit = did.changed, did.quit

	w.Close()
	os.Stderr = saved
	return <-done, cleared, quit
}

func TestQuitAsksToLeave(t *testing.T) {
	for _, line := range []string{"/quit", "/exit", "/q"} {
		_, cleared, quit := run(t, newAgent(), line, Options{})
		if !quit {
			t.Errorf("%s did not quit", line)
		}
		if cleared {
			t.Errorf("%s cleared the context", line)
		}
	}
}

func TestClearForgetsTheConversationAndItsUsage(t *testing.T) {
	for _, line := range []string{"/clear", "/new", "/reset"} {
		ag := newAgent()
		r, w, _ := os.Pipe()
		saved := os.Stderr
		os.Stderr = w
		usage := llm.Usage{InputTokens: 500, OutputTokens: 20}
		done := runCommand(ag, line, nil, &usage, Options{})
		cleared, quit := done.changed, done.quit
		w.Close()
		os.Stderr = saved
		io.Copy(io.Discard, r)

		if !cleared {
			t.Errorf("%s did not clear", line)
		}
		if quit {
			t.Errorf("%s quit", line)
		}
		if usage != (llm.Usage{}) {
			t.Errorf("%s left usage at %+v", line, usage)
		}
	}
}

func TestModelShowsAndSets(t *testing.T) {
	ag := newAgent()

	out, _, _ := run(t, ag, "/model", Options{})
	if !strings.Contains(out, "stub-1") {
		t.Errorf("/model did not report the default model:\n%s", out)
	}
	if !strings.Contains(out, "stub-2") {
		t.Errorf("/model did not list the known models:\n%s", out)
	}

	if _, _, _ = run(t, ag, "/model stub-2", Options{}); ag.Model != "stub-2" {
		t.Errorf("model = %q, want stub-2", ag.Model)
	}
}

func TestEffort(t *testing.T) {
	ag := newAgent()

	if _, _, _ = run(t, ag, "/effort high", Options{}); ag.Effort != llm.EffortHigh {
		t.Errorf("effort = %q, want high", ag.Effort)
	}
	if _, _, _ = run(t, ag, "/effort -", Options{}); ag.Effort != "" {
		t.Errorf("effort = %q, want the provider default", ag.Effort)
	}

	ag.Effort = llm.EffortMax
	out, _, _ := run(t, ag, "/effort sideways", Options{})
	if ag.Effort != llm.EffortMax {
		t.Errorf("a bad effort changed the setting to %q", ag.Effort)
	}
	if !strings.Contains(out, "sideways") {
		t.Errorf("the error does not quote the input:\n%s", out)
	}
}

func TestThinkingTogglesVisibility(t *testing.T) {
	ag := newAgent()

	if _, _, _ = run(t, ag, "/thinking on", Options{}); !ag.Thinking.Show {
		t.Error("/thinking on did not show it")
	}
	if _, _, _ = run(t, ag, "/thinking off", Options{}); ag.Thinking.Show {
		t.Error("/thinking off did not hide it")
	}

	out, _, _ := run(t, ag, "/thinking maybe", Options{})
	if !strings.Contains(out, "usage:") {
		t.Errorf("a bad argument did not explain itself:\n%s", out)
	}
}

func TestSystemSetsShowsAndDrops(t *testing.T) {
	ag := newAgent()

	out, _, _ := run(t, ag, "/system", Options{})
	if !strings.Contains(out, "(none)") {
		t.Errorf("an unset system prompt should read as none:\n%s", out)
	}

	if _, _, _ = run(t, ag, "/system be terse", Options{}); ag.System != "be terse" {
		t.Errorf("system = %q", ag.System)
	}
	if out, _, _ := run(t, ag, "/system", Options{}); !strings.Contains(out, "be terse") {
		t.Errorf("/system did not echo the prompt:\n%s", out)
	}
	if _, _, _ = run(t, ag, "/system -", Options{}); ag.System != "" {
		t.Errorf("system = %q, want empty", ag.System)
	}
}

func TestPermissionShowsAndSets(t *testing.T) {
	ag := newAgent()

	out, _, _ := run(t, ag, "/permission", Options{})
	if !strings.Contains(out, string(permission.Manual)) {
		t.Errorf("/permission did not report the mode:\n%s", out)
	}

	for _, line := range []string{"/permission plan", "/perm plan", "/mode plan"} {
		ag.Gate.Policy.SetMode(permission.Manual)
		if _, _, _ = run(t, ag, line, Options{}); ag.Gate.Mode() != permission.Plan {
			t.Errorf("%s left the mode at %q", line, ag.Gate.Mode())
		}
	}

	ag.Gate.Policy.SetMode(permission.Manual)
	out, _, _ = run(t, ag, "/permission sideways", Options{})
	if ag.Gate.Mode() != permission.Manual {
		t.Error("a bad mode was accepted")
	}
	if !strings.Contains(out, "sideways") {
		t.Errorf("the error does not quote the input:\n%s", out)
	}
}

func TestPermissionListsWhatWasGranted(t *testing.T) {
	ag := newAgent()
	ag.Gate.Policy.Remember(permission.Request{Tool: "bash", Action: permission.Execute, Target: "git status"})

	out, _, _ := run(t, ag, "/permission", Options{})
	if !strings.Contains(out, "bash git") {
		t.Errorf("/permission did not list the grant:\n%s", out)
	}
}

func TestPermissionWithNothingGated(t *testing.T) {
	ag := newAgent()
	ag.Gate = nil
	if out, _, _ := run(t, ag, "/permission", Options{}); !strings.Contains(out, "nothing is gated") {
		t.Errorf("got:\n%s", out)
	}
}

func TestToolsWithNoTools(t *testing.T) {
	if out, _, _ := run(t, newAgent(), "/tools", Options{}); !strings.Contains(out, "no tools") {
		t.Errorf("got:\n%s", out)
	}
}

func TestUsageReportsTheSessionTotals(t *testing.T) {
	ag := newAgent()
	r, w, _ := os.Pipe()
	saved := os.Stderr
	os.Stderr = w
	usage := llm.Usage{InputTokens: 1234, OutputTokens: 56}
	runCommand(ag, "/usage", nil, &usage, Options{})
	w.Close()
	os.Stderr = saved
	b, _ := io.ReadAll(r)

	out := string(b)
	for _, want := range []string{"1234", "56", "stub", "(not saved)"} {
		if !strings.Contains(out, want) {
			t.Errorf("/usage omits %q:\n%s", want, out)
		}
	}
}

func TestSessionsWithNoRepo(t *testing.T) {
	if out, _, _ := run(t, newAgent(), "/sessions", Options{}); !strings.Contains(out, "not being saved") {
		t.Errorf("got:\n%s", out)
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	out, _, _ := run(t, newAgent(), "/help", Options{})
	for _, want := range []string{"/clear", "/model", "/provider", "/quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("/help omits %s", want)
		}
	}
}

func TestAnUnknownCommandPointsAtHelp(t *testing.T) {
	out, _, quit := run(t, newAgent(), "/nonsense", Options{})
	if quit {
		t.Error("an unknown command quit")
	}
	if !strings.Contains(out, "/help") {
		t.Errorf("got:\n%s", out)
	}
}

func TestCommandNamesAreCaseInsensitive(t *testing.T) {
	ag := newAgent()
	if _, _, _ = run(t, ag, "/SYSTEM be terse", Options{}); ag.System != "be terse" {
		t.Errorf("system = %q", ag.System)
	}
}

func TestArgumentsAreTrimmed(t *testing.T) {
	ag := newAgent()
	if _, _, _ = run(t, ag, "/model   stub-2   ", Options{}); ag.Model != "stub-2" {
		t.Errorf("model = %q, want stub-2", ag.Model)
	}
}
