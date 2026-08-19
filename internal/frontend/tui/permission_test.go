package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zenodea/zaino/internal/permission"
)

func editRequest() permission.Request {
	return permission.Request{
		Tool:    "edit",
		Action:  permission.Write,
		Target:  "main.go",
		Preview: "    3 - old\n    3 + new",
	}
}

func TestApproverRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want permission.Grant
	}{
		{"y allows once", "y", permission.Once},
		{"enter allows once", "enter", permission.Once},
		{"a allows always", "a", permission.Always},
		{"n refuses", "n", permission.Reject},
		{"esc refuses", "esc", permission.Reject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, 80, 24)

			answered := make(chan permission.Grant, 1)
			go func() {
				grant, err := m.Approver().Approve(context.Background(), editRequest())
				if err != nil {
					t.Errorf("Approve: %v", err)
				}
				answered <- grant
			}()

			m.Update(<-m.events)
			if m.pending == nil {
				t.Fatal("the question never reached the model")
			}

			send(m, pressKey(tt.key))
			if m.pending != nil {
				t.Error("the question is still open after being answered")
			}

			select {
			case got := <-answered:
				if got != tt.want {
					t.Errorf("grant = %v, want %v", got, tt.want)
				}
			case <-time.After(time.Second):
				t.Fatal("the agent was left waiting")
			}
		})
	}
}

func pressKey(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

func TestPendingQuestionOwnsTheKeyboard(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.pending = &pendingAsk{req: editRequest(), reply: make(chan permission.Grant, 1)}

	send(m, pressKey("h"), pressKey("i"))
	if got := m.input.Value(); got != "" {
		t.Errorf("input = %q, want the keys to have gone to the question", got)
	}
}

func TestCancelWhileWaitingRefusesFirst(t *testing.T) {
	m := newTestModel(t, 80, 24)

	cancelled := false
	m.cancel = func() { cancelled = true }
	m.streaming = true

	answered := make(chan permission.Grant, 1)
	go func() {
		grant, _ := m.Approver().Approve(context.Background(), editRequest())
		answered <- grant
	}()

	m.Update(<-m.events)
	send(m, pressKey("ctrl+c"))

	select {
	case got := <-answered:
		if got != permission.Reject {
			t.Errorf("grant = %v, want Reject", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the agent was left waiting after ⌃c")
	}
	if !cancelled {
		t.Error("the turn was not cancelled")
	}
}

func TestApproveGivesUpWhenTheTurnIsCancelled(t *testing.T) {
	m := newTestModel(t, 80, 24)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := m.Approver().Approve(ctx, editRequest())
		done <- err
	}()

	m.Update(<-m.events)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Approve returned no error after the context went away")
		}
	case <-time.After(time.Second):
		t.Fatal("Approve did not give up")
	}
}

func TestAskViewShowsTheChange(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.pending = &pendingAsk{req: editRequest(), reply: make(chan permission.Grant, 1)}

	view := stripANSI(m.askView())
	for _, want := range []string{"Write", "main.go", "- old", "+ new", "y allow", "n refuse"} {
		if !strings.Contains(view, want) {
			t.Errorf("askView() is missing %q:\n%s", want, view)
		}
	}
}

func TestModeCyclesOnShiftTab(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.Gate = &permission.Gate{Policy: permission.NewPolicy(permission.Manual)}

	send(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := m.agent.Gate.Mode(); got == permission.Manual {
		t.Errorf("mode = %q, want it to have moved on", got)
	}
}

func TestAskPanelWidthIsStable(t *testing.T) {
	m := newTestModel(t, 90, 24)

	requests := []permission.Request{
		{Tool: "edit", Action: permission.Write, Target: "a.go", Preview: "    1 - x\n    1 + y"},
		{Tool: "bash", Action: permission.Execute, Target: "ls", Preview: "ls"},
		{Tool: "write", Action: permission.Write, Target: strings.Repeat("deep/", 40) + "f.go"},
	}

	widths := map[int]bool{}
	for _, req := range requests {
		m.pending = &pendingAsk{req: req, reply: make(chan permission.Grant, 1)}
		view := m.askView()
		for _, line := range strings.Split(view, "\n") {
			widths[lipgloss.Width(line)] = true
		}
	}

	if len(widths) != 1 {
		t.Errorf("panel lines came out at widths %v, want one width throughout", keysOf(widths))
	}
	for w := range widths {
		if w > m.contentWidth() {
			t.Errorf("panel is %d wide, wider than the %d available", w, m.contentWidth())
		}
	}
}

func keysOf(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func TestNarrowTerminalKeepsEveryKey(t *testing.T) {
	m := newTestModel(t, 44, 24)
	m.pending = &pendingAsk{
		req:   permission.Request{Tool: "bash", Action: permission.Execute, Target: "rm -rf build"},
		reply: make(chan permission.Grant, 1),
	}

	view := stripANSI(m.askView())
	for _, key := range []string{"y", "a", "n"} {
		if !strings.Contains(view, key+" ") {
			t.Errorf("key %q missing at 44 columns:\n%s", key, view)
		}
	}
}

func TestTabReadsTheWholeChange(t *testing.T) {
	m := newTestModel(t, 80, 24)
	req := editRequest()
	var long []string
	for i := range 40 {
		long = append(long, fmt.Sprintf("+ line %d", i))
	}
	req.Preview = strings.Join(long, "\n")
	reply := make(chan permission.Grant, 1)
	m.pending = &pendingAsk{req: req, reply: reply}

	if !strings.Contains(stripANSI(m.askView()), "24 more lines") {
		t.Fatalf("the panel should say what it is hiding:\n%s", stripANSI(m.askView()))
	}

	send(m, pressKey("tab"))
	if !m.sheet.open || !m.sheet.ask {
		t.Fatal("tab did not open the change on a sheet")
	}
	if len(m.sheet.lines) != 40 {
		t.Errorf("sheet holds %d lines, want all 40", len(m.sheet.lines))
	}
	if m.pending == nil {
		t.Fatal("reading the change must not answer the question")
	}

	send(m, pressKey("G"))
	if m.sheet.offset == 0 {
		t.Error("G should scroll to the end")
	}

	send(m, pressKey("y"))
	if m.pending != nil || m.sheet.open {
		t.Error("answering from the sheet should answer and close it")
	}
	if got := <-reply; got != permission.Once {
		t.Errorf("grant = %v, want Once", got)
	}
}

func TestEscLeavesTheSheetNotTheQuestion(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.pending = &pendingAsk{req: editRequest(), reply: make(chan permission.Grant, 1)}

	send(m, pressKey("tab"), pressKey("esc"))
	if m.sheet.open {
		t.Error("esc should close the sheet")
	}
	if m.pending == nil {
		t.Error("esc from the sheet must not refuse the question")
	}
}
