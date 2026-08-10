package anthropic

import (
	"io"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func sseBody(blocks ...string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(strings.Join(blocks, "\n\n") + "\n\n"))
}

const (
	evMessageStart = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":25,"output_tokens":1,"cache_read_input_tokens":10}}}`

	evMessageStop = `event: message_stop
data: {"type":"message_stop"}`
)

func TestStreamAccumulatesToolUse(t *testing.T) {
	body := sseBody(
		evMessageStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me "}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"check."}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`: keep-alive comment`,
		`event: ping
data: {"type":"ping"}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_9","name":"read","input":{}}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"th\": \"/etc/hosts\"}"}}`,
		`data: {"type":"content_block_stop","index":1}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":57}}`,
		evMessageStop,
	)

	stream := newStream(body)
	defer stream.Close()

	var text strings.Builder
	events := 0
	for stream.Next() {
		events++
		if d, ok := stream.Event().(llm.ContentBlockDeltaEvent); ok {
			if td, ok := d.Delta.(llm.TextDelta); ok {
				text.WriteString(td.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if events == 0 {
		t.Fatal("no events decoded")
	}
	if got := text.String(); got != "Let me check." {
		t.Errorf("streamed text = %q", got)
	}

	msg := stream.Message()
	if msg.StopReason != llm.StopToolUse {
		t.Errorf("stop reason = %q, want tool_use", msg.StopReason)
	}
	if msg.ID != "msg_1" || msg.Model != "claude-opus-5" {
		t.Errorf("message header not carried from message_start: %+v", msg)
	}
	if msg.Usage.InputTokens != 25 || msg.Usage.OutputTokens != 57 || msg.Usage.CacheReadTokens != 10 {
		t.Errorf("usage = %+v", msg.Usage)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("got %d content blocks, want 2", len(msg.Content))
	}
	if got := msg.Text(); got != "Let me check." {
		t.Errorf("accumulated text = %q", got)
	}

	uses := msg.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("got %d tool uses, want 1", len(uses))
	}
	if uses[0].ID != "toolu_9" || uses[0].Name != "read" {
		t.Errorf("tool use = %+v", uses[0])
	}
	if got := string(uses[0].Input); got != `{"path": "/etc/hosts"}` {
		t.Errorf("assembled input = %s", got)
	}
}

func TestStreamAccumulatesThinking(t *testing.T) {
	body := sseBody(
		evMessageStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"step one "}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"step two"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"SIG=="}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}`,
		evMessageStop,
	)

	stream := newStream(body)
	defer stream.Close()
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	msg := stream.Message()
	block, ok := msg.Content[0].(llm.ThinkingBlock)
	if !ok {
		t.Fatalf("got %T, want llm.ThinkingBlock", msg.Content[0])
	}
	if block.Thinking != "step one step two" {
		t.Errorf("thinking = %q", block.Thinking)
	}
	if block.Signature != "SIG==" {
		t.Errorf("signature = %q", block.Signature)
	}
}

func TestStreamErrorEvent(t *testing.T) {
	body := sseBody(
		evMessageStart,
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
	)

	stream := newStream(body)
	defer stream.Close()
	for stream.Next() {
	}
	err := stream.Err()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.Type != "overloaded_error" {
		t.Errorf("type = %q", apiErr.Type)
	}
}

func TestStreamTruncated(t *testing.T) {
	body := sseBody(
		evMessageStart,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
	)

	stream := newStream(body)
	defer stream.Close()
	for stream.Next() {
	}
	if stream.Err() == nil {
		t.Fatal("want truncation error, got nil")
	}
	if got := stream.Message().Text(); got != "partial" {
		t.Errorf("partial text should still be accumulated, got %q", got)
	}
}
