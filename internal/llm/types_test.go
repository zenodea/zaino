package llm

import (
	"encoding/json"
	"testing"
)

func TestContentRoundTrip(t *testing.T) {
	original := []byte(`[` +
		`{"type":"text","text":"hello"},` +
		`{"type":"thinking","thinking":"pondering","signature":"sig-abc"},` +
		`{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"/tmp/x"}},` +
		`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"go"}}` +
		`]`)

	var content Content
	if err := json.Unmarshal(original, &content); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(content) != 4 {
		t.Fatalf("got %d blocks, want 4", len(content))
	}

	if _, ok := content[0].(TextBlock); !ok {
		t.Errorf("block 0: got %T, want TextBlock", content[0])
	}
	think, ok := content[1].(ThinkingBlock)
	if !ok {
		t.Fatalf("block 1: got %T, want ThinkingBlock", content[1])
	}
	if think.Signature != "sig-abc" {
		t.Errorf("signature = %q, want %q", think.Signature, "sig-abc")
	}
	use, ok := content[2].(ToolUseBlock)
	if !ok {
		t.Fatalf("block 2: got %T, want ToolUseBlock", content[2])
	}
	if got := string(use.Input); got != `{"path":"/tmp/x"}` {
		t.Errorf("input = %s", got)
	}

	unknown, ok := content[3].(OpaqueBlock)
	if !ok {
		t.Fatalf("block 3: got %T, want OpaqueBlock", content[3])
	}
	if unknown.Type != "server_tool_use" {
		t.Errorf("unknown type = %q", unknown.Type)
	}

	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !jsonEqual(t, encoded, original) {
		t.Errorf("round trip mismatch:\n got %s\nwant %s", encoded, original)
	}
}

func TestToolResultMarshal(t *testing.T) {
	got, err := json.Marshal(ToolResultBlock{ToolUseID: "toolu_1", Content: "42", IsError: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"type":"tool_result","tool_use_id":"toolu_1","content":"42","is_error":true}`)
	if !jsonEqual(t, got, want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestToolUseEmptyInput(t *testing.T) {
	got, err := json.Marshal(ToolUseBlock{ID: "toolu_1", Name: "now"})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"type":"tool_use","id":"toolu_1","name":"now","input":{}}`)
	if !jsonEqual(t, got, want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("unmarshal a: %v (%s)", err, a)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("unmarshal b: %v (%s)", err, b)
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}
