package tui

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
)

func TestCustomCommandSendsItsPrompt(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.UseConfig(&config.Config{Commands: []config.Command{
		{Name: "review", Description: "read the diff", Prompt: "Review $ARGUMENTS", Path: "/tmp/review.md"},
	}})

	if _, ok := findCommand(m.commands(), "review"); !ok {
		t.Fatal("/review is not in the registry")
	}
	m.runCommand("/review the gate")

	if len(m.messages) != 1 || m.messages[0].Text() != "Review the gate" {
		t.Fatalf("messages = %+v", m.messages)
	}
	// The transcript shows what was asked for, not the whole of what was sent.
	last := m.entries[len(m.entries)-1]
	if last.kind != entryUser || !strings.Contains(last.text, "/review") {
		t.Errorf("entry = %+v", last)
	}
}

func TestCustomCommandCannotTakeABuiltInName(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.UseConfig(&config.Config{Commands: []config.Command{
		{Name: "quit", Prompt: "not today", Path: "/tmp/quit.md"},
	}})

	if len(m.custom) != 0 {
		t.Fatal("a file took a name zaino already answers to")
	}
	m.runCommand("/quit")
	if !m.quitting {
		t.Error("/quit stopped meaning quit")
	}
}

func TestBroNeedsSomethingToSimplify(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.UseConfig(&config.Config{Commands: config.Builtins()})

	m.runCommand("/bro")
	if len(m.messages) != 0 {
		t.Fatal("/bro sent a prompt with nothing said yet")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "bro") {
		t.Errorf("entry = %+v", last)
	}

	m.messages = append(m.messages,
		llm.UserText("why"), llm.Message{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "because"}}})
	m.runCommand("/bro")

	if len(m.messages) != 3 || !strings.Contains(m.messages[2].Text(), "Re-explain") {
		t.Fatalf("messages = %+v", m.messages)
	}
}

func TestProfileSwitchesTheModel(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.UseConfig(&config.Config{File: config.File{Profiles: map[string]config.Profile{
		"cheap": {Model: "claude-haiku-4-5", Effort: llm.EffortLow},
	}}})

	m.runCommand("/profile cheap")
	if m.agent.Model != "claude-haiku-4-5" || m.agent.Effort != llm.EffortLow {
		t.Errorf("model = %q, effort = %q", m.agent.Model, m.agent.Effort)
	}

	m.runCommand("/profile nope")
	if last := m.entries[len(m.entries)-1]; last.kind != entryError {
		t.Errorf("an unknown profile passed quietly: %+v", last)
	}
}

func TestAPromptCanPointAtAPicture(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	m := newTestModel(t, 80, 24)
	typeLine(m, "why is @shot.png wrong")
	m.submit()

	if len(m.messages) != 1 {
		t.Fatalf("messages = %+v", m.messages)
	}
	content := m.messages[0].Content
	if len(content) != 2 {
		t.Fatalf("content = %+v, want the text and the picture", content)
	}
	if _, ok := content[1].(llm.ImageBlock); !ok {
		t.Errorf("content[1] = %T", content[1])
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "shot.png") {
		t.Errorf("the transcript does not say what was attached: %q", last.text)
	}
}

// The turn does not go out at all, since a prompt about a screenshot that
// arrives without one reads as though the model went blind.
func TestAPictureThatIsNotThereStopsTheTurn(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newTestModel(t, 80, 24)

	typeLine(m, "why is @missing.png wrong")
	m.submit()

	if len(m.messages) != 0 {
		t.Fatalf("messages = %+v, want nothing sent", m.messages)
	}
	if last := m.entries[len(m.entries)-1]; last.kind != entryError {
		t.Errorf("entry = %+v, want an error", last)
	}
}
