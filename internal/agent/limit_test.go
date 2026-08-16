package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider/anthropic"
)

// A fake that answers both endpoints, so a turn can be stopped by what
// count_tokens says rather than by what the estimator guesses.
type countingAPI struct {
	turns []string

	mu     sync.Mutex
	count  int
	counts int
	sends  int
	bodies []json.RawMessage
}

func (f *countingAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		f.counts++
		f.bodies = append(f.bodies, json.RawMessage(body))
		count := f.count
		f.mu.Unlock()

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"input_tokens": count})
		return
	}

	n := f.sends
	f.sends++
	f.mu.Unlock()

	if n >= len(f.turns) {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"no more turns"}}`,
			http.StatusBadRequest)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	io.WriteString(w, f.turns[n])
}

func (f *countingAPI) tally() (counts, sends int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts, f.sends
}

func newCountingAgent(t *testing.T, count int, turns ...string) (*Agent, *countingAPI) {
	t.Helper()
	api := &countingAPI{turns: turns, count: count}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	client, err := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(server.URL),
		anthropic.WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &Agent{Provider: client, Model: "test-model", MaxTokens: 1024}, api
}

func TestContextLimitStopsBeforeAnythingIsSent(t *testing.T) {
	ag, api := newCountingAgent(t, 200_001, textTurn("hello"))
	ag.MaxContext = 200_000

	history := []llm.Message{llm.UserText(strings.Repeat("x", 4000))}
	out, err := ag.Run(context.Background(), history)

	var limit *ContextLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("err = %v, want *ContextLimitError", err)
	}
	if limit.Used != 200_001 || limit.Limit != 200_000 || !limit.Exact {
		t.Errorf("limit = %+v, want 200001 of 200000 exact", limit)
	}
	if len(out) != len(history) {
		t.Errorf("history grew to %d messages; the turn should not have run", len(out))
	}

	counts, sends := api.tally()
	if counts != 1 {
		t.Errorf("count_tokens called %d times, want 1", counts)
	}
	if sends != 0 {
		t.Errorf("the turn was sent %d times, want 0", sends)
	}
}

// One token over is over: the point of asking the provider is that 200_000
// means 200_000.
func TestContextLimitAllowsExactlyTheLimit(t *testing.T) {
	ag, api := newCountingAgent(t, 200_000, textTurn("hello"))
	ag.MaxContext = 200_000

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText(strings.Repeat("x", 4000))}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, sends := api.tally(); sends != 1 {
		t.Errorf("sends = %d, want 1", sends)
	}
}

// A turn that reports a nearly full window of its own, so what follows it has
// to be measured rather than waved through.
func heavyTurn(text string, input int) string {
	return sseTurn(
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":`+strconv.Itoa(input)+`,"output_tokens":1}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+quote(text)+`}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
		turnStop,
	)
}

func TestAllowOnceSendsPastTheLimitAndIsSpent(t *testing.T) {
	ag, api := newCountingAgent(t, 200_001, heavyTurn("first", 210_000), textTurn("second"))
	ag.MaxContext = 200_000

	history := []llm.Message{llm.UserText(strings.Repeat("x", 4000))}

	ag.AllowOnce()
	history, err := ag.Run(context.Background(), history)
	if err != nil {
		t.Fatalf("Run after AllowOnce: %v", err)
	}
	if _, sends := api.tally(); sends != 1 {
		t.Errorf("sends = %d, want 1", sends)
	}

	history = append(history, llm.UserText("again"))
	if _, err := ag.Run(context.Background(), history); !errors.As(err, new(*ContextLimitError)) {
		t.Fatalf("second run err = %v, want the limit back", err)
	}
}

func TestNoLimitLeavesTheProviderUnasked(t *testing.T) {
	ag, api := newCountingAgent(t, 999_999, textTurn("hello"))

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("hi")}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if counts, _ := api.tally(); counts != 0 {
		t.Errorf("count_tokens called %d times with no limit set, want 0", counts)
	}
}

// A short conversation cannot reach a large ceiling however it is tokenised,
// so nothing is bought to prove it.
func TestSmallHistorySkipsTheCount(t *testing.T) {
	ag, api := newCountingAgent(t, 1, textTurn("one"), textTurn("two"))
	ag.MaxContext = 200_000

	history, err := ag.Run(context.Background(), []llm.Message{llm.UserText("hi")})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if counts, _ := api.tally(); counts != 1 {
		t.Errorf("count_tokens called %d times on the first turn, want 1", counts)
	}

	// The turn reported its own numbers, so the second one has a baseline and
	// only a few bytes were added since.
	history = append(history, llm.UserText("and again"))
	if _, err := ag.Run(context.Background(), history); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if counts, _ := api.tally(); counts != 1 {
		t.Errorf("count_tokens called %d times overall, want 1", counts)
	}
}

func TestCountRequestCarriesNoReplyParameters(t *testing.T) {
	ag, api := newCountingAgent(t, 1, textTurn("hello"))
	ag.MaxContext = 200_000
	ag.System = "be brief"

	if _, err := ag.Run(context.Background(), []llm.Message{llm.UserText("hi")}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	api.mu.Lock()
	body := api.bodies[0]
	api.mu.Unlock()

	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"stream", "max_tokens"} {
		if _, ok := sent[unwanted]; ok {
			t.Errorf("count request carried %q", unwanted)
		}
	}
	if _, ok := sent["system"]; !ok {
		t.Error("count request left the system prompt out; it is part of the window")
	}
}

// Without a provider that counts, the ceiling still holds — it just says so in
// the words it can stand behind.
func TestLimitFallsBackToTheEstimate(t *testing.T) {
	ag := &Agent{Provider: &countlessProvider{}, Model: "test-model", MaxContext: 100}

	_, err := ag.Run(context.Background(), []llm.Message{llm.UserText(strings.Repeat("x", 4000))})

	var limit *ContextLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("err = %v, want *ContextLimitError", err)
	}
	if limit.Exact {
		t.Error("the estimate was reported as exact")
	}
	if !strings.Contains(limit.Error(), "about") {
		t.Errorf("Error() = %q, want it to admit the estimate", limit.Error())
	}
}

type countlessProvider struct{}

func (*countlessProvider) Name() string         { return "countless" }
func (*countlessProvider) DefaultModel() string { return "test-model" }
func (*countlessProvider) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return nil, errors.New("should not have been sent")
}

// Compaction rewrites the history the meter was counted against, so the next
// turn has to measure again rather than add to a stale number.
func TestCompactionForgetsTheMeter(t *testing.T) {
	ag := &Agent{Model: "test-model"}
	ag.remeasured(150_000, 4)
	ag.forget()

	if _, _, ok := ag.baseline(make([]llm.Message, 8)); ok {
		t.Error("the meter survived a rewrite of the history it counted")
	}
}

func TestBaselineIsDroppedWhenTheHistoryShrinks(t *testing.T) {
	ag := &Agent{Model: "test-model"}
	ag.remeasured(150_000, 8)

	if _, _, ok := ag.baseline(make([]llm.Message, 4)); ok {
		t.Error("a baseline covering more messages than there are was kept")
	}
	if _, _, ok := ag.baseline(make([]llm.Message, 8)); !ok {
		t.Error("a baseline covering the whole history was dropped")
	}
}

func TestBaselineIsDroppedWhenTheModelChanges(t *testing.T) {
	ag := &Agent{Model: "test-model"}
	ag.remeasured(150_000, 2)
	ag.Model = "other-model"

	if _, _, ok := ag.baseline(make([]llm.Message, 4)); ok {
		t.Error("a count made by another tokeniser was kept")
	}
}

// The skip is only sound while a token cannot be smaller than a byte.
func TestByteSizeBoundsTheEstimate(t *testing.T) {
	messages := []llm.Message{
		llm.UserText("hello there"),
		{Role: llm.RoleAssistant, Content: llm.Content{
			llm.ThinkingBlock{Thinking: strings.Repeat("t", 500)},
			llm.ToolUseBlock{ID: "toolu_1", Name: "read", Input: json.RawMessage(`{"path":"x"}`)},
		}},
		{Role: llm.RoleUser, Content: llm.Content{
			llm.ToolResultBlock{ToolUseID: "toolu_1", Content: strings.Repeat("r", 2000)},
		}},
	}
	if bytes, estimate := byteSize(messages), estimateTokens(messages); bytes <= estimate {
		t.Errorf("byteSize = %d, estimateTokens = %d; the bound is not an upper one", bytes, estimate)
	}
}

func TestContextTokensCountsTheWholeWindow(t *testing.T) {
	got := contextTokens(llm.Usage{
		InputTokens: 100, OutputTokens: 20, ThinkingTokens: 5,
		CacheReadTokens: 1000, CacheWriteTokens: 300,
	})
	if got != 1425 {
		t.Errorf("contextTokens = %d, want 1425; cached tokens sit in the window too", got)
	}
}

func TestParseTokens(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    int
		wantErr bool
	}{
		{in: "200k", want: 200_000},
		{in: "200000", want: 200_000},
		{in: "1M", want: 1_000_000},
		{in: "1.5k", want: 1500},
		{in: " 200K ", want: 200_000},
		{in: "off"},
		{in: "none"},
		{in: "0"},
		{in: "-5", wantErr: true},
		{in: "lots", wantErr: true},
		{in: "", wantErr: true},
	} {
		got, err := ParseTokens(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseTokens(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("ParseTokens(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
