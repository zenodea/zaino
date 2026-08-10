package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
)

type ToolFunc func(ctx context.Context, input json.RawMessage) (string, error)

type Tool struct {
	Definition llm.Tool
	Run        ToolFunc
}

type Hooks struct {
	OnTextDelta     func(text string)
	OnThinkingDelta func(text string)
	OnToolCall      func(call llm.ToolUseBlock)
	OnToolResult    func(call llm.ToolUseBlock, result string, isError bool)
	OnTurn          func(resp *llm.Response)
}

type Agent struct {
	Provider llm.Provider

	Model     string
	MaxTokens int
	System    string
	Effort    string
	Thinking  *llm.Thinking

	Tools []Tool

	MaxTurns int

	Hooks Hooks
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

func (a *Agent) Run(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	maxTurns := a.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.turn(ctx, history)
		if err != nil {
			return history, err
		}
		history = append(history, resp.ToMessage())

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

func (a *Agent) turn(ctx context.Context, history []llm.Message) (*llm.Response, error) {
	req := llm.Request{
		Model:     a.Model,
		MaxTokens: orDefault(a.MaxTokens, DefaultMaxTokens),
		System:    a.System,
		Messages:  history,
		Thinking:  a.Thinking,
		Effort:    a.Effort,
	}
	for _, t := range a.Tools {
		req.Tools = append(req.Tools, t.Definition)
	}

	stream, err := a.Provider.Stream(ctx, req)
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

	var wg sync.WaitGroup
	for i, call := range calls {
		if a.Hooks.OnToolCall != nil {
			a.Hooks.OnToolCall(call)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, isErr := a.invoke(ctx, call)
			results[i] = llm.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   out,
				IsError:   isErr,
			}
			if a.Hooks.OnToolResult != nil {
				a.Hooks.OnToolResult(call, out, isErr)
			}
		}()
	}
	wg.Wait()
	return results
}

func (a *Agent) invoke(ctx context.Context, call llm.ToolUseBlock) (out string, isErr bool) {
	tool, ok := a.lookup(call.Name)
	if !ok {
		return fmt.Sprintf("Error: no tool named %q is available.", call.Name), true
	}

	defer func() {
		if r := recover(); r != nil {
			out, isErr = fmt.Sprintf("Error: tool %q panicked: %v", call.Name, r), true
		}
	}()

	result, err := tool.Run(ctx, call.Input)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	return result, false
}

func (a *Agent) lookup(name string) (Tool, bool) {
	for _, t := range a.Tools {
		if t.Definition.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func orDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}
