package tui

import (
	"strings"
	"testing"
)

func TestToolCallsSitTogether(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.ready = true

	m.push(entry{kind: entryThinking, text: "looking"})
	for _, name := range []string{"read", "grep", "ls"} {
		m.push(entry{kind: entryTool, toolName: name, toolInput: `{"path":"a.go"}`, done: true})
	}
	send(m, textDeltaMsg("here is what I found"))

	lines := strings.Split(stripANSI(m.transcript()), "\n")
	activity := 0
	for _, line := range lines {
		if strings.Contains(line, "▸") || strings.Contains(line, "⋯") {
			activity++
			continue
		}
		if activity > 0 && activity < 4 && strings.TrimSpace(line) == "" {
			t.Errorf("a blank line broke up the run of activity:\n%s", strings.Join(lines, "\n"))
			return
		}
	}
}

func TestProseGetsAirAroundIt(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.ready = true

	m.push(entry{kind: entryTool, toolName: "read", toolInput: `{"path":"a.go"}`, done: true})
	send(m, textDeltaMsg("done"))

	lines := strings.Split(stripANSI(m.transcript()), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "done") {
			continue
		}
		if i == 0 || strings.TrimSpace(lines[i-1]) != "" {
			t.Errorf("no blank line between the tool run and the answer:\n%s",
				strings.Join(lines, "\n"))
		}
		return
	}
	t.Fatal("the answer is missing from the transcript")
}

func TestToolSummaries(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"read shows the path", "read", `{"path":"internal/agent/agent.go","offset":40}`, "internal/agent/agent.go"},
		{"write shows the path", "write", `{"path":"main.go","content":"package main"}`, "main.go"},
		{"bash shows the command", "bash", `{"command":"go test ./..."}`, "go test ./..."},
		{"grep shows the pattern", "grep", `{"pattern":"runTools"}`, `"runTools"`},
		{"grep shows its glob too", "grep", `{"pattern":"runTools","glob":"*.go"}`, `"runTools" in *.go`},
		{"find shows the pattern", "find", `{"pattern":"*_test.go"}`, `"*_test.go"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entry{kind: entryTool, toolName: tt.tool, toolInput: tt.input, done: true, resultLen: 100}
			if got := e.toolSummary(); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
			if line := e.toolLine(70); strings.Contains(line, `{"`) {
				t.Errorf("raw JSON leaked into the line: %q", line)
			}
		})
	}
}

func TestUnknownToolStillReads(t *testing.T) {
	e := entry{kind: entryTool, toolName: "deploy", toolInput: `{"target":"prod"}`,
		toolArgs: `{"target":"prod"}`, done: true, resultLen: 12}

	if line := e.toolLine(70); !strings.Contains(line, "deploy") || !strings.Contains(line, "prod") {
		t.Errorf("line = %q, want the tool and its argument", line)
	}
}

func TestToolStatusIsRightAligned(t *testing.T) {
	e := entry{kind: entryTool, toolName: "read", toolInput: `{"path":"a.go"}`, done: true, resultLen: 2048}

	line := e.toolLine(60)
	if len([]rune(line)) != 60 {
		t.Errorf("line is %d wide, want 60: %q", len([]rune(line)), line)
	}
	if !strings.HasSuffix(line, "2.0kB") {
		t.Errorf("line = %q, want the size at the right edge", line)
	}
}

func TestLongToolSummaryDoesNotPushOutTheStatus(t *testing.T) {
	e := entry{kind: entryTool, toolName: "read",
		toolInput: `{"path":"` + strings.Repeat("very/deep/", 20) + `file.go"}`,
		done:      true, resultLen: 2048}

	line := e.toolLine(50)
	if len([]rune(line)) > 50 {
		t.Errorf("line is %d wide, want at most 50", len([]rune(line)))
	}
	if !strings.HasSuffix(line, "2.0kB") {
		t.Errorf("the status was pushed off the end: %q", line)
	}
}
