package llm

import (
	"encoding/json"
	"testing"
)

func feed(a *Accumulator, events ...Event) {
	for _, ev := range events {
		a.Add(ev)
	}
}

func TestAccumulatorAssemblesText(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{ID: "msg_1", Model: "m", Role: RoleAssistant}},
		ContentBlockStartEvent{Index: 0, Block: TextBlock{}},
		ContentBlockDeltaEvent{Index: 0, Delta: TextDelta{Text: "Hello, "}},
		ContentBlockDeltaEvent{Index: 0, Delta: TextDelta{Text: "world"}},
		ContentBlockStopEvent{Index: 0},
		MessageDeltaEvent{StopReason: StopEndTurn},
		MessageStopEvent{},
	)

	resp := a.Response()
	if resp.ID != "msg_1" {
		t.Errorf("id = %q, want msg_1", resp.ID)
	}
	if got := resp.Text(); got != "Hello, world" {
		t.Errorf("text = %q, want %q", got, "Hello, world")
	}
	if resp.StopReason != StopEndTurn {
		t.Errorf("stop reason = %q, want end_turn", resp.StopReason)
	}
	if !a.Stopped() {
		t.Error("Stopped() = false after message_stop")
	}
}

func TestAccumulatorIsNotStoppedUntilTheStreamSaysSo(t *testing.T) {
	var a Accumulator
	feed(&a, MessageStartEvent{Message: Response{ID: "msg_1"}}, PingEvent{})
	if a.Stopped() {
		t.Error("Stopped() = true before message_stop")
	}
}

func TestAccumulatorAssemblesToolInputFromFragments(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{Role: RoleAssistant}},
		ContentBlockStartEvent{Index: 0, Block: ToolUseBlock{ID: "toolu_1", Name: "read"}},
		ContentBlockDeltaEvent{Index: 0, Delta: InputJSONDelta{PartialJSON: `{"pa`}},
		ContentBlockDeltaEvent{Index: 0, Delta: InputJSONDelta{PartialJSON: `th":"/e`}},
		ContentBlockDeltaEvent{Index: 0, Delta: InputJSONDelta{PartialJSON: `tc/hosts"}`}},
		ContentBlockStopEvent{Index: 0},
		MessageDeltaEvent{StopReason: StopToolUse},
		MessageStopEvent{},
	)

	uses := a.Response().ToolUses()
	if len(uses) != 1 {
		t.Fatalf("got %d tool uses, want 1", len(uses))
	}
	if uses[0].ID != "toolu_1" || uses[0].Name != "read" {
		t.Errorf("got %+v", uses[0])
	}
	if got := string(uses[0].Input); got != `{"path":"/etc/hosts"}` {
		t.Errorf("input = %s", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal(uses[0].Input, &decoded); err != nil {
		t.Fatalf("input is not valid json: %v", err)
	}
}

// A tool call with no arguments streams no deltas at all.
func TestAToolUseWithNoDeltasKeepsItsInput(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		ContentBlockStartEvent{Index: 0, Block: ToolUseBlock{ID: "toolu_1", Name: "now", Input: json.RawMessage(`{}`)}},
		ContentBlockStopEvent{Index: 0},
	)
	uses := a.Response().ToolUses()
	if len(uses) != 1 || string(uses[0].Input) != `{}` {
		t.Fatalf("got %+v", uses)
	}
}

func TestAccumulatorAssemblesThinkingAndItsSignature(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		ContentBlockStartEvent{Index: 0, Block: ThinkingBlock{}},
		ContentBlockDeltaEvent{Index: 0, Delta: ThinkingDelta{Thinking: "let me "}},
		ContentBlockDeltaEvent{Index: 0, Delta: ThinkingDelta{Thinking: "count"}},
		ContentBlockDeltaEvent{Index: 0, Delta: SignatureDelta{Signature: "sig-"}},
		ContentBlockDeltaEvent{Index: 0, Delta: SignatureDelta{Signature: "abc"}},
		ContentBlockStopEvent{Index: 0},
	)

	block, ok := a.Response().Content[0].(ThinkingBlock)
	if !ok {
		t.Fatalf("block 0 is %T, want ThinkingBlock", a.Response().Content[0])
	}
	if block.Thinking != "let me count" {
		t.Errorf("thinking = %q", block.Thinking)
	}
	if block.Signature != "sig-abc" {
		t.Errorf("signature = %q, want sig-abc", block.Signature)
	}
}

func TestAccumulatorKeepsBlocksInIndexOrder(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		ContentBlockStartEvent{Index: 0, Block: ThinkingBlock{Thinking: "hmm"}},
		ContentBlockStopEvent{Index: 0},
		ContentBlockStartEvent{Index: 1, Block: TextBlock{}},
		ContentBlockDeltaEvent{Index: 1, Delta: TextDelta{Text: "answer"}},
		ContentBlockStopEvent{Index: 1},
		ContentBlockStartEvent{Index: 2, Block: ToolUseBlock{ID: "toolu_1", Name: "bash"}},
		ContentBlockDeltaEvent{Index: 2, Delta: InputJSONDelta{PartialJSON: `{"cmd":"ls"}`}},
		ContentBlockStopEvent{Index: 2},
		MessageStopEvent{},
	)

	content := a.Response().Content
	if len(content) != 3 {
		t.Fatalf("got %d blocks, want 3", len(content))
	}
	if _, ok := content[0].(ThinkingBlock); !ok {
		t.Errorf("block 0 = %T", content[0])
	}
	if _, ok := content[1].(TextBlock); !ok {
		t.Errorf("block 1 = %T", content[1])
	}
	if _, ok := content[2].(ToolUseBlock); !ok {
		t.Errorf("block 2 = %T", content[2])
	}
	if got := a.Response().Text(); got != "answer" {
		t.Errorf("text = %q, want answer — thinking must not leak into it", got)
	}
}

func TestAccumulatorGrowsForASkippedIndex(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		ContentBlockStartEvent{Index: 2, Block: TextBlock{Text: "late"}},
	)
	content := a.Response().Content
	if len(content) != 3 {
		t.Fatalf("got %d blocks, want 3", len(content))
	}
	if content[0] != nil || content[1] != nil {
		t.Errorf("gaps should be nil, got %v %v", content[0], content[1])
	}
}

// Gemini repeats cumulative usage on every chunk; summing would double-count.
func TestUsageTakesTheHighWaterMarkNotTheSum(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		MessageDeltaEvent{Usage: Usage{InputTokens: 100, OutputTokens: 10, ThinkingTokens: 4, CacheReadTokens: 7, CacheWriteTokens: 2}},
		MessageDeltaEvent{Usage: Usage{InputTokens: 100, OutputTokens: 25, ThinkingTokens: 9, CacheReadTokens: 7, CacheWriteTokens: 2}},
		MessageStopEvent{},
	)

	want := Usage{InputTokens: 100, OutputTokens: 25, ThinkingTokens: 9, CacheReadTokens: 7, CacheWriteTokens: 2}
	if got := a.Response().Usage; got != want {
		t.Errorf("usage = %+v, want %+v", got, want)
	}
}

func TestALaterDeltaNeverLowersUsage(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		MessageDeltaEvent{Usage: Usage{OutputTokens: 40}},
		MessageDeltaEvent{Usage: Usage{OutputTokens: 0}},
	)
	if got := a.Response().Usage.OutputTokens; got != 40 {
		t.Errorf("output = %d, want 40", got)
	}
}

func TestAnEmptyStopReasonDoesNotOverwriteAnEarlierOne(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		MessageDeltaEvent{StopReason: StopMaxTokens},
		MessageDeltaEvent{Usage: Usage{OutputTokens: 1}},
	)
	if got := a.Response().StopReason; got != StopMaxTokens {
		t.Errorf("stop reason = %q, want max_tokens", got)
	}
}

func TestStopDetailsAreCarried(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		MessageDeltaEvent{StopReason: StopRefusal, StopDetails: &StopDetails{Category: "policy", Explanation: "no"}},
	)
	got := a.Response().StopDetails
	if got == nil || got.Category != "policy" {
		t.Fatalf("stop details = %+v", got)
	}
}

func TestADeltaForTheWrongBlockTypeIsIgnored(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{}},
		ContentBlockStartEvent{Index: 0, Block: TextBlock{Text: "kept"}},
		ContentBlockDeltaEvent{Index: 0, Delta: ThinkingDelta{Thinking: "stray"}},
		ContentBlockDeltaEvent{Index: 0, Delta: SignatureDelta{Signature: "stray"}},
		ContentBlockDeltaEvent{Index: 0, Delta: OpaqueDelta{Type: "citation"}},
	)
	block, ok := a.Response().Content[0].(TextBlock)
	if !ok {
		t.Fatalf("block 0 = %T", a.Response().Content[0])
	}
	if block.Text != "kept" {
		t.Errorf("text = %q, want kept", block.Text)
	}
}

func TestStrayEventsDoNotPanic(t *testing.T) {
	var a Accumulator
	feed(&a,
		ContentBlockStopEvent{Index: 9},
		ContentBlockDeltaEvent{Index: 3, Delta: TextDelta{Text: "orphan"}},
		PingEvent{},
		MessageStopEvent{},
	)
	if !a.Stopped() {
		t.Error("Stopped() = false")
	}
}

// A stream that starts with no content array must still be appendable.
func TestMessageStartWithoutContentIsUsable(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{ID: "msg_1"}},
		ContentBlockStartEvent{Index: 0, Block: TextBlock{Text: "hi"}},
	)
	if got := a.Response().Text(); got != "hi" {
		t.Errorf("text = %q, want hi", got)
	}
}

func TestASecondMessageStartResetsTheResponse(t *testing.T) {
	var a Accumulator
	feed(&a,
		MessageStartEvent{Message: Response{ID: "msg_1"}},
		ContentBlockStartEvent{Index: 0, Block: TextBlock{Text: "first"}},
		MessageStartEvent{Message: Response{ID: "msg_2"}},
	)
	resp := a.Response()
	if resp.ID != "msg_2" {
		t.Errorf("id = %q, want msg_2", resp.ID)
	}
	if len(resp.Content) != 0 {
		t.Errorf("content = %v, want empty", resp.Content)
	}
}
