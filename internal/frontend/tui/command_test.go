package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
)

func typeLine(m *Model, line string) {
	m.input.SetValue(line)
	m.syncInputChrome()
}

func lastEntry(t *testing.T, m *Model) entry {
	t.Helper()
	if len(m.entries) == 0 {
		t.Fatal("no entries")
	}
	return m.entries[len(m.entries)-1]
}

func TestCommandLineDetection(t *testing.T) {
	cases := map[string]bool{
		"/help":                 true,
		"/model claude-opus-5":  true,
		"/q":                    true,
		"/etc/hosts is broken":  false,
		"//":                    false,
		"/":                     false,
		"what does / mean":      false,
		"/2fa":                  true,
		"read ./main.go please": false,
	}
	for line, want := range cases {
		if got := isCommandLine(line); got != want {
			t.Errorf("isCommandLine(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestSubmitRunsCommandInsteadOfSending(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.messages = []llm.Message{llm.UserText("earlier")}
	typeLine(m, "/clear")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.streaming {
		t.Error("a command must not start a turn")
	}
	if len(m.messages) != 0 {
		t.Errorf("history not cleared: %+v", m.messages)
	}
	if got := lastEntry(t, m); got.kind != entryNotice {
		t.Errorf("want a notice, got %+v", got)
	}
}

type stubProvider struct{}

func (stubProvider) Name() string         { return "stub" }
func (stubProvider) DefaultModel() string { return "stub-1" }

func (stubProvider) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return nil, errors.New("stub: offline")
}

func TestSubmitSendsPathLikePrompt(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.agent.Provider = stubProvider{}
	typeLine(m, "/etc/hosts is broken")

	m.submit()

	if len(m.messages) != 1 || m.messages[0].Text() != "/etc/hosts is broken" {
		t.Errorf("prompt should have been sent, history = %+v", m.messages)
	}
}

func TestUnknownCommandReports(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.runCommand("/nope")

	got := lastEntry(t, m)
	if got.kind != entryError || !strings.Contains(got.text, "/nope") {
		t.Errorf("want an error naming the command, got %+v", got)
	}
	if len(m.messages) != 0 {
		t.Errorf("nothing should reach the model, history = %+v", m.messages)
	}
}

func TestModelCommandShowsAndSets(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.lastModel = "claude-opus-5"

	m.runCommand("/model")
	if got := lastEntry(t, m); !strings.Contains(got.text, "claude-opus-5") {
		t.Errorf("want the current model, got %q", got.text)
	}

	m.runCommand("/model claude-haiku-4-5")
	if m.agent.Model != "claude-haiku-4-5" {
		t.Errorf("agent model = %q", m.agent.Model)
	}
	if m.lastModel != "" {
		t.Error("the model reported by the last turn should be dropped")
	}
	if !strings.Contains(m.header(), "claude-haiku-4-5") {
		t.Errorf("header should follow the change: %q", m.header())
	}
}

func TestEffortCommandValidates(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.runCommand("/effort high")
	if m.agent.Effort != llm.EffortHigh {
		t.Errorf("effort = %q", m.agent.Effort)
	}

	m.runCommand("/effort turbo")
	if got := lastEntry(t, m); got.kind != entryError {
		t.Errorf("want an error for an unknown level, got %+v", got)
	}
	if m.agent.Effort != llm.EffortHigh {
		t.Errorf("a rejected level must not stick: %q", m.agent.Effort)
	}

	m.runCommand("/effort -")
	if m.agent.Effort != "" {
		t.Errorf("effort should be cleared, got %q", m.agent.Effort)
	}
}

func TestThinkingCommandToggles(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.runCommand("/thinking on")
	if m.agent.Thinking == nil || !m.agent.Thinking.Show {
		t.Fatalf("thinking = %+v", m.agent.Thinking)
	}
	m.runCommand("/thinking off")
	if m.agent.Thinking.Show {
		t.Error("thinking should be hidden")
	}
}

func TestSystemCommandSetsAndDrops(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.runCommand("/system be terse")
	if m.agent.System != "be terse" {
		t.Errorf("system = %q", m.agent.System)
	}
	m.runCommand("/system -")
	if m.agent.System != "" {
		t.Errorf("system should be dropped, got %q", m.agent.System)
	}
}

func TestUsageCommandReportsSession(t *testing.T) {
	m := newTestModel(t, 80, 24)
	send(m, turnMsg{Usage: llm.Usage{InputTokens: 1200, OutputTokens: 340}})

	m.runCommand("/usage")

	text := lastEntry(t, m).text
	for _, want := range []string{"1200", "340", "anthropic"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage report missing %q:\n%s", want, text)
		}
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.runCommand("/help")

	text := lastEntry(t, m).text
	for _, c := range commandList() {
		if !strings.Contains(text, "/"+c.name) {
			t.Errorf("help omits /%s:\n%s", c.name, text)
		}
	}
}

func TestQuitCommand(t *testing.T) {
	m := newTestModel(t, 80, 24)
	if cmd := m.runCommand("/exit"); cmd == nil {
		t.Fatal("/exit should return a quit command")
	}
	if !m.quitting {
		t.Error("model should be quitting")
	}
}

func TestFuzzyMatchRanksBestFirst(t *testing.T) {
	cases := map[string]string{
		"":     "help",
		"cl":   "clear",
		"mo":   "model",
		"er":   "effort",
		"prov": "provider",
		"q":    "quit", // via the /q alias
		"thk":  "thinking",
	}
	for pattern, want := range cases {
		matches := matchCommands(pattern)
		if len(matches) == 0 {
			t.Errorf("%q matched nothing", pattern)
			continue
		}
		if matches[0].name != want {
			t.Errorf("%q ranked /%s first, want /%s", pattern, matches[0].name, want)
		}
	}
	if got := matchCommands("zzz"); len(got) != 0 {
		t.Errorf("nonsense should match nothing, got %+v", got)
	}
}

func TestMenuOpensFiltersAndCompletes(t *testing.T) {
	m := newTestModel(t, 80, 24)

	typeLine(m, "hello")
	if m.menu.open {
		t.Error("plain text must not open the menu")
	}

	typeLine(m, "/")
	if !m.menu.open || len(m.menu.matches) != len(commandList()) {
		t.Fatalf("menu = %+v", m.menu)
	}

	typeLine(m, "/mod")
	if len(m.menu.matches) == 0 || m.menu.matches[0].name != "model" {
		t.Fatalf("matches = %+v", m.menu.matches)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "/model " {
		t.Errorf("tab should complete to %q, got %q", "/model ", m.input.Value())
	}
	if m.menu.open {
		t.Error("a trailing argument closes the menu")
	}
}

func TestMenuNavigationAndEnter(t *testing.T) {
	m := newTestModel(t, 80, 24)
	typeLine(m, "/")

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.menu.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.menu.cursor)
	}
	if m.menu.matches[1].name != "clear" {
		t.Fatalf("second match = %s", m.menu.matches[1].name)
	}

	m.messages = []llm.Message{llm.UserText("earlier")}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.messages) != 0 {
		t.Errorf("enter should have run the highlighted /clear, history = %+v", m.messages)
	}
	if m.input.Value() != "" {
		t.Errorf("input should be cleared, got %q", m.input.Value())
	}
	if m.menu.open {
		t.Error("menu should close after running")
	}
}

func TestMenuKeepsSelectionWhileNarrowing(t *testing.T) {
	m := newTestModel(t, 80, 24)
	typeLine(m, "/")
	m.moveMenu(2) // /model

	typeLine(m, "/m")
	if got, _ := m.selected(); got.name != "model" {
		t.Errorf("selection = /%s, want /model", got.name)
	}
}

func TestMenuEscapeAndStreaming(t *testing.T) {
	m := newTestModel(t, 80, 24)
	typeLine(m, "/")

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.menu.open {
		t.Error("escape should dismiss the menu")
	}

	m.streaming = true
	typeLine(m, "/cl")
	if m.menu.open {
		t.Error("the menu must stay shut mid-turn")
	}
}

func TestMenuTakesRoomFromTheViewport(t *testing.T) {
	m := newTestModel(t, 80, 24)
	tall := m.viewport.Height

	typeLine(m, "/")
	short := m.viewport.Height
	if short >= tall {
		t.Errorf("viewport should shrink for the panel: %d then %d", tall, short)
	}
	if got := strings.Count(m.View(), "\n") + 1; got > 24 {
		t.Errorf("view is %d lines with the menu open, want <= 24", got)
	}

	typeLine(m, "hello")
	if m.viewport.Height != tall {
		t.Errorf("viewport should return to %d, got %d", tall, m.viewport.Height)
	}
}
