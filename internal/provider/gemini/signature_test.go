package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

const wireSignature = "CtsBAcu98PBsig"

func TestThoughtSignatureIsReadFromFunctionCall(t *testing.T) {
	s := &stream{}
	events := s.translatePart(part{
		FunctionCall:     &functionCall{Name: "ls", Args: map[string]any{"path": "."}},
		ThoughtSignature: wireSignature,
	})

	for _, ev := range events {
		start, ok := ev.(llm.ContentBlockStartEvent)
		if !ok {
			continue
		}
		use, ok := start.Block.(llm.ToolUseBlock)
		if !ok {
			t.Fatalf("block = %T, want a tool use", start.Block)
		}
		if use.Signature != wireSignature {
			t.Errorf("Signature = %q, want %q", use.Signature, wireSignature)
		}
		return
	}
	t.Fatal("no content block start event")
}

func TestThoughtSignatureIsSentBack(t *testing.T) {
	msg := llm.Message{
		Role: llm.RoleAssistant,
		Content: llm.Content{llm.ToolUseBlock{
			ID:        "synthetic-0-ls",
			Name:      "ls",
			Input:     json.RawMessage(`{"path":"."}`),
			Signature: wireSignature,
		}},
	}

	out, err := translateMessage(msg, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(out.Parts))
	}
	if out.Parts[0].ThoughtSignature != wireSignature {
		t.Errorf("ThoughtSignature = %q, want %q", out.Parts[0].ThoughtSignature, wireSignature)
	}
	if out.Parts[0].FunctionCall == nil {
		t.Fatal("the function call went missing")
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"thoughtSignature":"`+wireSignature+`"`) {
		t.Errorf("wire JSON is missing the signature: %s", raw)
	}
}

func TestSignatureSurvivesTheAccumulator(t *testing.T) {
	var acc llm.Accumulator
	s := &stream{}

	for _, ev := range s.translatePart(part{
		FunctionCall:     &functionCall{Name: "ls", Args: map[string]any{"path": "."}},
		ThoughtSignature: wireSignature,
	}) {
		acc.Add(ev)
	}

	uses := acc.Response().ToolUses()
	if len(uses) != 1 {
		t.Fatalf("got %d tool uses, want 1", len(uses))
	}
	if uses[0].Signature != wireSignature {
		t.Errorf("Signature = %q, want it to survive accumulation", uses[0].Signature)
	}
}
