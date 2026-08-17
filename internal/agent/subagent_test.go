package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

func stubTool(name string) tool.Tool {
	return &tool.Func{
		Def: llm.Tool{Name: name},
		Do:  func(context.Context, json.RawMessage) (string, error) { return "", nil },
	}
}

func prepareTask(t *testing.T, ag *Agent, args map[string]string) (*taskCall, error) {
	t.Helper()
	input, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	call, err := TaskTool(ag).Prepare(input)
	if err != nil {
		return nil, err
	}
	return call.(*taskCall), nil
}

func TestTaskOffersTheNamedAgents(t *testing.T) {
	ag := &Agent{Subagents: []Subagent{{Name: "scout", Description: "finds things"}}}
	def := TaskTool(ag).Definition()

	if !strings.Contains(def.Description, "scout: finds things") {
		t.Errorf("description does not offer the agent:\n%s", def.Description)
	}
	properties := def.InputSchema["properties"].(map[string]any)
	agent, ok := properties["agent"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no agent field")
	}
	if names, _ := agent["enum"].([]string); len(names) != 1 || names[0] != "scout" {
		t.Errorf("enum = %v", agent["enum"])
	}

	// With none defined the field is not there to be guessed at.
	if _, offered := TaskTool(&Agent{}).Definition().InputSchema["properties"].(map[string]any)["agent"]; offered {
		t.Error("an agent field appeared with no agents defined")
	}
}

func TestTaskTakesANamedAgent(t *testing.T) {
	ag := &Agent{
		System:    "the parent's prompt",
		Model:     "parent-model",
		Subagents: []Subagent{{Name: "scout", System: "you search", Model: "small", Tools: []string{"read"}}},
		Tools:     []tool.Tool{stubTool("read"), stubTool("bash")},
	}

	call, err := prepareTask(t, ag, map[string]string{"description": "look", "prompt": "find it", "agent": "scout"})
	if err != nil {
		t.Fatal(err)
	}
	if call.agent.Name != "scout" {
		t.Fatalf("agent = %+v", call.agent)
	}

	tools, err := call.agent.toolbox(ag.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if names := tool.Names(tools); len(names) != 1 || names[0] != "read" {
		t.Errorf("tools = %v, want only what the agent asked for", names)
	}
}

func TestTaskRefusesAnAgentThatIsNotThere(t *testing.T) {
	ag := &Agent{Subagents: []Subagent{{Name: "scout"}}}

	_, err := prepareTask(t, ag, map[string]string{"description": "look", "prompt": "find it", "agent": "sccout"})
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "scout") {
		t.Errorf("error does not say what there is: %v", err)
	}
}

func TestSubagentInheritsTheToolsItDoesNotName(t *testing.T) {
	all := []tool.Tool{stubTool("read"), stubTool("bash")}
	tools, err := Subagent{Name: "scout"}.toolbox(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != len(all) {
		t.Errorf("tools = %v, want the lot", tool.Names(tools))
	}

	if _, err := (Subagent{Tools: []string{"nosuch"}}).toolbox(all); err == nil {
		t.Error("a tool that does not exist went unreported")
	}
}

// Where zaino is running goes out with the system prompt, but is not one:
// it is read fresh, so it must not be mistaken for what the session recorded.
func TestProjectContextRidesWithTheSystemPrompt(t *testing.T) {
	ag := &Agent{System: "be terse", Project: "this repo is zaino"}
	if got := ag.request(nil).System; got != "be terse\n\nthis repo is zaino" {
		t.Errorf("system = %q", got)
	}

	if got := (&Agent{Project: "alone"}).request(nil).System; got != "alone" {
		t.Errorf("system = %q, want no leading blank", got)
	}
	if got := (&Agent{System: "alone"}).request(nil).System; got != "alone" {
		t.Errorf("system = %q", got)
	}
}

// A tool result is text on every provider, so what the model has to look at
// follows the results in the same message rather than sitting inside one.
func TestAttachmentsFollowTheResults(t *testing.T) {
	picture := llm.ImageBlock{MediaType: "image/png", Data: []byte("bytes")}
	ag := &Agent{Tools: []tool.Tool{&shows{image: picture}}}

	content := ag.runTools(context.Background(), []llm.ToolUseBlock{{ID: "toolu_a", Name: "shows"}})

	if len(content) != 2 {
		t.Fatalf("content = %+v, want the result and the picture", content)
	}
	if _, ok := content[0].(llm.ToolResultBlock); !ok {
		t.Errorf("content[0] = %T, want the result first", content[0])
	}
	if got, ok := content[1].(llm.ImageBlock); !ok || string(got.Data) != "bytes" {
		t.Errorf("content[1] = %+v", content[1])
	}
}

type shows struct{ image llm.ImageBlock }

func (s *shows) Definition() llm.Tool { return llm.Tool{Name: "shows"} }

func (s *shows) Prepare(json.RawMessage) (tool.Call, error) { return s, nil }

func (s *shows) Request() permission.Request {
	return permission.Request{Tool: "shows", Action: permission.Read}
}

func (s *shows) Run(context.Context) (string, error) { return "a picture, below", nil }

func (s *shows) Attachments() llm.Content { return llm.Content{s.image} }
