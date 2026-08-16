package llm

import (
	"encoding/json"
	"testing"
)

func TestUserText(t *testing.T) {
	m := UserText("hello")
	if m.Role != RoleUser {
		t.Errorf("role = %q, want user", m.Role)
	}
	if got := m.Text(); got != "hello" {
		t.Errorf("text = %q", got)
	}
}

func TestMessageTextJoinsOnlyTextBlocks(t *testing.T) {
	m := Message{Role: RoleAssistant, Content: Content{
		ThinkingBlock{Thinking: "hidden"},
		TextBlock{Text: "one "},
		ToolUseBlock{ID: "toolu_1", Name: "read"},
		TextBlock{Text: "two"},
		OpaqueBlock{Type: "citation", Raw: json.RawMessage(`{"type":"citation"}`)},
	}}
	if got := m.Text(); got != "one two" {
		t.Errorf("text = %q, want %q", got, "one two")
	}
}

func TestMessageTextOfNothing(t *testing.T) {
	if got := (Message{}).Text(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestMessageToolUses(t *testing.T) {
	m := Message{Content: Content{
		TextBlock{Text: "sure"},
		ToolUseBlock{ID: "toolu_1", Name: "read"},
		ToolUseBlock{ID: "toolu_2", Name: "bash"},
	}}
	uses := m.ToolUses()
	if len(uses) != 2 {
		t.Fatalf("got %d, want 2", len(uses))
	}
	if uses[0].Name != "read" || uses[1].Name != "bash" {
		t.Errorf("got %v", uses)
	}
	if got := (Message{}).ToolUses(); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResponseToMessageDefaultsToAssistant(t *testing.T) {
	r := Response{Content: Content{TextBlock{Text: "hi"}}}
	if got := r.ToMessage().Role; got != RoleAssistant {
		t.Errorf("role = %q, want assistant", got)
	}

	r.Role = RoleUser
	if got := r.ToMessage().Role; got != RoleUser {
		t.Errorf("role = %q, want the role it was given", got)
	}
}

func TestResponseReadsThroughToItsMessage(t *testing.T) {
	r := Response{Content: Content{
		TextBlock{Text: "answer"},
		ToolUseBlock{ID: "toolu_1", Name: "grep"},
	}}
	if got := r.Text(); got != "answer" {
		t.Errorf("text = %q", got)
	}
	if got := r.ToolUses(); len(got) != 1 || got[0].Name != "grep" {
		t.Errorf("tool uses = %v", got)
	}
}

func TestBlockTypesAreDistinct(t *testing.T) {
	blocks := []ContentBlock{
		TextBlock{}, ThinkingBlock{}, ToolUseBlock{}, ToolResultBlock{},
		OpaqueBlock{Type: "server_tool_use"},
	}
	want := []string{"text", "thinking", "tool_use", "tool_result", "server_tool_use"}
	seen := map[string]bool{}
	for i, b := range blocks {
		got := b.blockType()
		if got != want[i] {
			t.Errorf("block %d: got %q, want %q", i, got, want[i])
		}
		if seen[got] {
			t.Errorf("duplicate block type %q", got)
		}
		seen[got] = true
	}
}

func TestContentRejectsMalformedJSON(t *testing.T) {
	var c Content
	if err := c.UnmarshalJSON([]byte(`{"not":"an array"}`)); err == nil {
		t.Fatal("got nil, want an error")
	}
	if err := c.UnmarshalJSON([]byte(`[{"type":"text","text":123}]`)); err == nil {
		t.Fatal("got nil, want an error for a bad block")
	}
}

func TestOpaqueBlockSurvivesUntouched(t *testing.T) {
	raw := json.RawMessage(`{"type":"web_search_result","url":"https://example.com"}`)
	b, err := UnmarshalBlock(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(raw) {
		t.Errorf("got %s, want %s", out, raw)
	}
}
