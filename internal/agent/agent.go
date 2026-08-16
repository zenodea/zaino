package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

type Hooks struct {
	OnTextDelta     func(text string)
	OnThinkingDelta func(text string)
	OnToolCall      func(call llm.ToolUseBlock)
	OnToolResult    func(call llm.ToolUseBlock, result string, isError bool)
	OnTurn          func(resp *llm.Response)
	OnCompact       func(summary string, kept []llm.Message)
}

type Agent struct {
	Provider llm.Provider

	Model     string
	MaxTokens int
	System    string
	Effort    string
	Thinking  *llm.Thinking

	Tools      []tool.Tool
	Gate       *permission.Gate
	Compaction *Compaction

	MaxTurns  int
	TaskTurns int

	// A hard ceiling on the context, in tokens. Zero leaves the session
	// unbounded. Unlike the compaction window nothing is folded to stay under
	// it: the turn stops instead, and only AllowOnce gets past.
	MaxContext int

	Hooks Hooks

	// What the provider counted for the last turn, so the next one knows how
	// close to the window it is.
	used  int
	meter meter

	allowOnce bool
}

const (
	DefaultMaxTokens = 64000
	DefaultMaxTurns  = 32
)

var ErrMaxTurns = errors.New("agent: exceeded max turns")

var ErrTruncated = errors.New("agent: response truncated at max_tokens")

type RefusalError struct {
	Details *llm.StopDetails
}

func (e *RefusalError) Error() string {
	if e.Details == nil {
		return "agent: request refused"
	}
	return fmt.Sprintf("agent: request refused (%s): %s", e.Details.Category, e.Details.Explanation)
}

// What the provider counted for the last turn, and how much it may hold.
func (a *Agent) Used() int { return a.used }

// Ceiling reports the tightest bound on this session, the hard limit if one is
// set and the compaction window otherwise.
func (a *Agent) Ceiling() int {
	if a.MaxContext > 0 && (a.Window() == 0 || a.MaxContext < a.Window()) {
		return a.MaxContext
	}
	return a.Window()
}

func (a *Agent) Window() int {
	if a.Compaction == nil {
		return 0
	}
	return a.Compaction.window()
}

func (a *Agent) Run(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	maxTurns := a.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	// Consent to go over covers the whole run: a tool loop that stopped to ask
	// again between a call and its result would be asking mid-sentence.
	allowed := a.allowOnce
	a.allowOnce = false

	for turn := 0; turn < maxTurns; turn++ {
		if a.shouldCompact(history) {
			compacted, err := a.compact(ctx, history)
			if err != nil {
				return history, err
			}
			history = compacted
		}

		if !allowed {
			if err := a.withinLimit(ctx, history); err != nil {
				return history, err
			}
		}

		resp, err := a.turn(ctx, history)
		if err != nil {
			return history, err
		}
		history = append(history, resp.ToMessage())
		a.used = contextTokens(resp.Usage)
		a.remeasured(a.used, len(history))

		if a.Hooks.OnTurn != nil {
			a.Hooks.OnTurn(resp)
		}

		switch resp.StopReason {
		case llm.StopToolUse:
			results := a.runTools(ctx, resp.ToolUses())
			history = append(history, llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			})

		case llm.StopPauseTurn:
			// Re-send unchanged; the server resumes its own tool loop.

		case llm.StopRefusal:
			return history, &RefusalError{Details: resp.StopDetails}

		case llm.StopMaxTokens:
			return history, ErrTruncated

		default:
			return history, nil
		}
	}
	return history, ErrMaxTurns
}

func (a *Agent) request(history []llm.Message) llm.Request {
	req := llm.Request{
		Model:     a.Model,
		MaxTokens: orDefault(a.MaxTokens, DefaultMaxTokens),
		System:    a.System,
		Messages:  history,
		Thinking:  a.Thinking,
		Effort:    a.Effort,
	}
	for _, t := range a.Tools {
		req.Tools = append(req.Tools, t.Definition())
	}
	return req
}

func (a *Agent) turn(ctx context.Context, history []llm.Message) (*llm.Response, error) {
	stream, err := a.Provider.Stream(ctx, a.request(history))
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	for stream.Next() {
		delta, ok := stream.Event().(llm.ContentBlockDeltaEvent)
		if !ok {
			continue
		}
		switch d := delta.Delta.(type) {
		case llm.TextDelta:
			if a.Hooks.OnTextDelta != nil {
				a.Hooks.OnTextDelta(d.Text)
			}
		case llm.ThinkingDelta:
			if a.Hooks.OnThinkingDelta != nil {
				a.Hooks.OnThinkingDelta(d.Thinking)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return stream.Message(), nil
}

// Every result must travel in one message; splitting them stops the model
// making parallel calls.
func (a *Agent) runTools(ctx context.Context, calls []llm.ToolUseBlock) llm.Content {
	results := make(llm.Content, len(calls))
	admitted := make([]tool.Call, len(calls))

	// Asking has to happen one at a time, or a batch of calls would put several
	// questions on screen at once.
	for i, call := range calls {
		if a.Hooks.OnToolCall != nil {
			a.Hooks.OnToolCall(call)
		}
		ready, err := a.admit(ctx, call)
		if err != nil {
			results[i] = a.result(call, "Error: "+err.Error(), true)
			continue
		}
		admitted[i] = ready
	}

	var wg sync.WaitGroup
	for i, ready := range admitted {
		if ready == nil || ready.Request().Action != permission.Read {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = a.execute(ctx, calls[i], ready)
		}()
	}
	wg.Wait()

	for i, ready := range admitted {
		if ready == nil || ready.Request().Action == permission.Read {
			continue
		}
		results[i] = a.execute(ctx, calls[i], ready)
	}
	return results
}

func (a *Agent) admit(ctx context.Context, call llm.ToolUseBlock) (ready tool.Call, err error) {
	t, ok := a.lookup(call.Name)
	if !ok {
		return nil, fmt.Errorf("no tool named %q is available", call.Name)
	}

	defer func() {
		if r := recover(); r != nil {
			ready, err = nil, fmt.Errorf("tool %q panicked while preparing: %v", call.Name, r)
		}
	}()

	if ready, err = t.Prepare(call.Input); err != nil {
		return nil, err
	}
	if err := a.Gate.Check(ctx, ready.Request()); err != nil {
		return nil, err
	}
	return ready, nil
}

func (a *Agent) execute(ctx context.Context, call llm.ToolUseBlock, ready tool.Call) (block llm.ToolResultBlock) {
	defer func() {
		if r := recover(); r != nil {
			block = a.result(call, fmt.Sprintf("Error: tool %q panicked: %v", call.Name, r), true)
		}
	}()

	out, err := ready.Run(ctx)
	if err != nil {
		return a.result(call, "Error: "+err.Error(), true)
	}
	return a.result(call, out, false)
}

func (a *Agent) result(call llm.ToolUseBlock, out string, isErr bool) llm.ToolResultBlock {
	if a.Hooks.OnToolResult != nil {
		a.Hooks.OnToolResult(call, out, isErr)
	}
	return llm.ToolResultBlock{ToolUseID: call.ID, Content: out, IsError: isErr}
}

func (a *Agent) lookup(name string) (tool.Tool, bool) {
	for _, t := range a.Tools {
		if t.Definition().Name == name {
			return t, true
		}
	}
	return nil, false
}

func orDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}
