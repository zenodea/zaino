package hook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fire(t *testing.T, hooks []Hook, p Payload) Outcome {
	t.Helper()
	s := New(t.TempDir(), hooks)
	s.Session = "s1"
	return s.Fire(context.Background(), p)
}

func TestAHookSeesThePayloadAndSpeaksOnStdout(t *testing.T) {
	out := fire(t, []Hook{{Event: PreTool, Run: `cat; echo; echo "tool is $ZAINO_TOOL in $ZAINO_SESSION"`}},
		Payload{Event: PreTool, Tool: "bash", Input: json.RawMessage(`{"command":"ls"}`)})
	if out.Blocked != "" || len(out.Failed) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	said := strings.Join(out.Said, "\n")
	for _, want := range []string{`"event":"pre-tool"`, `"command":"ls"`, `"session":"s1"`, "tool is bash in s1"} {
		if !strings.Contains(said, want) {
			t.Errorf("missing %q in %q", want, said)
		}
	}
}

func TestExitTwoBlocksWithTheReason(t *testing.T) {
	out := fire(t, []Hook{
		{Event: PreTool, Tools: []string{"bash"}, Run: `echo "not here" >&2; exit 2`},
		{Event: PreTool, Run: `echo never`},
	}, Payload{Event: PreTool, Tool: "bash"})
	if out.Blocked != "not here" {
		t.Errorf("blocked = %q", out.Blocked)
	}
	if len(out.Said) != 0 {
		t.Errorf("a later hook ran after the block: %v", out.Said)
	}
}

func TestOnlyMatchingHooksRun(t *testing.T) {
	out := fire(t, []Hook{
		{Event: PreTool, Tools: []string{"edit", "write"}, Run: `echo edits`},
		{Event: PostTool, Run: `echo after`},
	}, Payload{Event: PreTool, Tool: "bash"})
	if len(out.Said) != 0 || out.Blocked != "" {
		t.Errorf("outcome = %+v, want nothing to have run", out)
	}
	out = fire(t, []Hook{{Event: PreTool, Tools: []string{"edit"}, Run: `echo edits`}}, Payload{Event: PreTool, Tool: "edit"})
	if len(out.Said) != 1 || out.Said[0] != "edits" {
		t.Errorf("said = %v", out.Said)
	}
}

func TestAFailingHookIsReportedNotFatal(t *testing.T) {
	out := fire(t, []Hook{
		{Event: TurnEnd, Run: `echo broke >&2; exit 1`},
		{Event: TurnEnd, Run: `echo still ran`},
	}, Payload{Event: TurnEnd})
	if len(out.Failed) != 1 || !strings.Contains(out.Failed[0].Error(), "broke") {
		t.Errorf("failed = %v", out.Failed)
	}
	if len(out.Said) != 1 {
		t.Errorf("said = %v, want the second hook to have run", out.Said)
	}
}

func TestATimeoutIsAFailure(t *testing.T) {
	out := fire(t, []Hook{{Event: TurnEnd, Run: `sleep 5`, Timeout: 50 * time.Millisecond}}, Payload{Event: TurnEnd})
	if len(out.Failed) != 1 || !strings.Contains(out.Failed[0].Error(), "timed out") {
		t.Errorf("failed = %v", out.Failed)
	}
}

func TestHooksRunInTheProjectDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out := New(dir, []Hook{{Event: SessionStart, Run: `ls marker`}}).Fire(context.Background(), Payload{Event: SessionStart})
	if len(out.Said) != 1 || out.Said[0] != "marker" {
		t.Errorf("outcome = %+v", out)
	}
}

func TestANilSetIsQuiet(t *testing.T) {
	var s *Set
	if out := s.Fire(context.Background(), Payload{Event: PreTool}); out.Blocked != "" || len(out.Failed) != 0 {
		t.Errorf("outcome = %+v", out)
	}
	if s.Has(PreTool) || s.List() != nil {
		t.Error("a nil set has nothing")
	}
}
