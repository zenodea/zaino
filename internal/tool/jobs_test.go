package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func runTool(t *testing.T, tl Tool, input string) (string, error) {
	t.Helper()
	call, err := tl.Prepare(json.RawMessage(input))
	if err != nil {
		t.Fatalf("prepare %s: %v", input, err)
	}
	return call.Run(context.Background())
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s: still not so after 3s", what)
}

func TestABackgroundCommandCanBeReadAndStopped(t *testing.T) {
	t.Cleanup(StopJobs)
	w, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	out, err := runTool(t, &Bash{w}, `{"command": "echo hello; sleep 30", "background": true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "started j") {
		t.Fatalf("bash said %q, want a job id", out)
	}
	id := strings.Fields(strings.TrimPrefix(out, "started "))[0]

	eventually(t, "the job's output", func() bool {
		got, _ := runTool(t, &Job{}, `{"id": "`+id+`"}`)
		return strings.Contains(got, "hello") && strings.Contains(got, "running")
	})

	list, err := runTool(t, &Job{}, `{"action": "list"}`)
	if err != nil || !strings.Contains(list, id) {
		t.Errorf("list = %q, %v", list, err)
	}

	got, err := runTool(t, &Job{}, `{"id": "`+id+`", "action": "kill"}`)
	if err != nil || !strings.Contains(got, "stopped") {
		t.Errorf("kill = %q, %v", got, err)
	}
	eventually(t, "the job to be gone", func() bool {
		j, _ := jobs.get(id)
		return !j.running()
	})
	if got, _ := runTool(t, &Job{}, `{"id": "`+id+`"}`); !strings.Contains(got, "hello") || strings.Contains(got, "running") {
		t.Errorf("after kill = %q", got)
	}
}

func TestAnUnknownJobIsSaidSo(t *testing.T) {
	if _, err := runTool(t, &Job{}, `{"id": "j999"}`); err == nil {
		t.Error("want an error for a job that never was")
	}
	if _, err := (&Job{}).Prepare(json.RawMessage(`{"action": "dance", "id": "j1"}`)); err == nil {
		t.Error("want an error for an action that is not one")
	}
}

func TestStopJobsTakesEverythingDown(t *testing.T) {
	w, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runTool(t, &Bash{w}, `{"command": "sleep 30", "background": true}`); err != nil {
		t.Fatal(err)
	}
	StopJobs()
	for _, j := range Jobs() {
		if j.Running {
			t.Errorf("%s still running after StopJobs", j.ID)
		}
	}
}

func TestClipTailKeepsTheEnd(t *testing.T) {
	got := clipTail(strings.Repeat("a", 10)+"END", 5)
	if !strings.HasSuffix(got, "aaEND") || !strings.Contains(got, "cut") {
		t.Errorf("got %q", got)
	}
}
