package repl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/attach"
	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/wirelog"
)

type Options struct {
	Provider     string
	Config       *config.Config
	ShowThinking bool
	Verbose      bool

	Interactive bool
	Gate        *permission.Gate

	Repo     session.Repo
	Recorder *session.Recorder
	Restored session.Context
	Wire     *wirelog.Log
}

func Run(ag *agent.Agent, o Options) error {
	stdout := bufio.NewWriter(os.Stdout)
	defer stdout.Flush()

	ag.Hooks = agent.Hooks{
		OnTextDelta: func(text string) {
			stdout.WriteString(text)
			stdout.Flush()
		},
		OnToolCall: func(call llm.ToolUseBlock) {
			fmt.Fprintf(os.Stderr, "\n\x1b[2m· %s %s\x1b[0m\n", call.Name, compactJSON(call.Input))
		},
		OnToolResult: func(call llm.ToolUseBlock, result string, isError bool) {
			status := "ok"
			if isError {
				status = "error"
			}
			fmt.Fprintf(os.Stderr, "\x1b[2m· %s → %s (%d bytes)\x1b[0m\n", call.Name, status, len(result))
		},
	}
	if o.ShowThinking {
		ag.Hooks.OnThinkingDelta = func(text string) {
			fmt.Fprintf(os.Stderr, "\x1b[2m%s\x1b[0m", text)
		}
	}
	usage := o.Restored.Usage
	ag.Hooks.OnTurn = func(resp *llm.Response) {
		u := resp.Usage
		addUsage(&usage, u)

		o.Recorder.Turn(u)
		o.Wire.Turn(resp.Model, resp.StopReason, u)

		if o.Verbose {
			fmt.Fprintf(os.Stderr,
				"\n\x1b[2m[%s in=%d out=%d think=%d cache_read=%d stop=%s]\x1b[0m\n",
				resp.Model, u.InputTokens, u.OutputTokens, u.ThinkingTokens,
				u.CacheReadTokens, resp.StopReason)
		}
	}

	ag.Hooks.OnCompact = func(summary string, kept []llm.Message) {
		o.Recorder.Compact(summary, kept)
		fmt.Fprintf(os.Stderr, "\x1b[2m[compacted, %d messages kept]\x1b[0m\n", max(len(kept)-1, 0))
	}

	var taskMu sync.Mutex
	taskInfo := map[string]agent.TaskInfo{}
	taskUsage := map[string]*llm.Usage{}

	ag.Hooks.OnTaskDone = func(id string, history []llm.Message, err error) {
		taskMu.Lock()
		defer taskMu.Unlock()
		info := taskInfo[id]
		var spent llm.Usage
		if u := taskUsage[id]; u != nil {
			spent = *u
		}

		state := "done"
		if err != nil {
			state = "failed"
		}
		fmt.Fprintf(os.Stderr, "\x1b[2m⚒ %s %s\x1b[0m\n", info.Description, state)

		if recErr := o.Recorder.Append(session.Task(session.TaskBody{
			ID:          id,
			Description: info.Description,
			Agent:       info.Agent,
			Model:       info.Model,
			Depth:       info.Depth,
			Failed:      err != nil,
			Messages:    history,
			Usage:       spent,
		})); recErr != nil {
			fmt.Fprintln(os.Stderr, "\x1b[31msession not being saved: "+recErr.Error()+"\x1b[0m")
		}
	}

	var onTask func(info agent.TaskInfo) agent.Hooks
	onTask = func(info agent.TaskInfo) agent.Hooks {
		taskMu.Lock()
		taskInfo[info.ID] = info
		taskUsage[info.ID] = &llm.Usage{}
		taskMu.Unlock()
		fmt.Fprintf(os.Stderr, "\n\x1b[2m⚒ %s started\x1b[0m\n", info.Description)

		label := info.Description
		return agent.Hooks{
			OnToolCall: func(call llm.ToolUseBlock) {
				fmt.Fprintf(os.Stderr, "\x1b[2m· [%s] %s %s\x1b[0m\n", label, call.Name, compactJSON(call.Input))
			},
			OnToolResult: func(call llm.ToolUseBlock, result string, isError bool) {
				status := "ok"
				if isError {
					status = "error"
				}
				fmt.Fprintf(os.Stderr, "\x1b[2m· [%s] %s → %s (%d bytes)\x1b[0m\n", label, call.Name, status, len(result))
			},
			OnTurn: func(resp *llm.Response) {
				taskMu.Lock()
				defer taskMu.Unlock()
				if u := taskUsage[info.ID]; u != nil {
					addUsage(u, resp.Usage)
				}
				addUsage(&usage, resp.Usage)
			},
			OnTask:     onTask,
			OnTaskDone: ag.Hooks.OnTaskDone,
		}
	}
	ag.Hooks.OnTask = onTask

	interrupts := newInterrupts()
	defer interrupts.stop()

	messages := o.Restored.Messages
	if id := o.Recorder.ID(); id != "" && o.Verbose {
		fmt.Fprintf(os.Stderr, "\x1b[2m[session %s, %d messages]\x1b[0m\n", id, len(messages))
	}
	in := bufio.NewReader(os.Stdin)
	if o.Interactive && o.Gate != nil {
		o.Gate.Approver = &approver{in: in, rec: o.Recorder}
	}

	for {
		fmt.Fprintf(os.Stderr, "\x1b[1m› \x1b[0m")
		line, err := in.ReadString('\n')
		if errors.Is(err, io.EOF) {
			if strings.TrimSpace(line) == "" {
				fmt.Fprintln(os.Stderr)
				return nil
			}

			defer fmt.Fprintln(os.Stderr)
			messages = runTurn(ag, interrupts, messages, strings.TrimSpace(line), stdout, o.Recorder, nil)
			return nil
		}
		if err != nil {
			return err
		}

		prompt := strings.TrimSpace(line)
		if prompt == "" {
			continue
		}
		if isCommand(prompt) {
			done := runCommand(ag, prompt, messages, &usage, o)
			if done.quit {
				return nil
			}
			if done.changed {
				messages = done.context
			}
			if done.send == "" {
				continue
			}
			prompt = done.send
		}
		asker := in
		if !o.Interactive {
			asker = nil
		}
		messages = runTurn(ag, interrupts, messages, prompt, stdout, o.Recorder, asker)
	}
}

func runTurn(ag *agent.Agent, interrupts *interrupts, messages []llm.Message,
	prompt string, stdout *bufio.Writer, rec *session.Recorder, in *bufio.Reader) []llm.Message {
	content, attached, err := attach.Prompt(workdir(), prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\x1b[31m"+err.Error()+"\x1b[0m")
		return messages
	}
	for _, what := range attached {
		fmt.Fprintf(os.Stderr, "\x1b[2m⧉ %s\x1b[0m\n", what)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: content})

	for {
		ctx, cancel := context.WithCancel(context.Background())
		interrupts.bind(cancel)
		updated, err := ag.Run(ctx, messages)
		interrupts.unbind()
		cancel()
		messages = updated

		if saveErr := rec.Messages(messages); saveErr != nil {
			fmt.Fprintln(os.Stderr, "\x1b[31msession not being saved: "+saveErr.Error()+"\x1b[0m")
		}

		stdout.Flush()
		fmt.Fprintln(os.Stderr)

		var overLimit *agent.ContextLimitError
		switch {
		case err == nil:
		case errors.As(err, &overLimit):
			if !askPastLimit(overLimit, in) {
				return messages
			}
			ag.AllowOnce()
			continue
		case errors.Is(err, context.Canceled):
			fmt.Fprintln(os.Stderr, "\x1b[2minterrupted\x1b[0m")
		default:
			fmt.Fprintln(os.Stderr, "\x1b[31m"+err.Error()+"\x1b[0m")
		}
		return messages
	}
}

// Nothing was sent, so the conversation is still whole either way; the only
// question is whether to go over. Without a terminal to ask, it does not.
func askPastLimit(err *agent.ContextLimitError, in *bufio.Reader) bool {
	about := "about "
	if err.Exact {
		about = ""
	}
	fmt.Fprintf(os.Stderr, "\x1b[31m⚠ context limit · %s%d tokens of %d\x1b[0m\n",
		about, err.Used, err.Limit)
	if in == nil {
		fmt.Fprintln(os.Stderr, "\x1b[2mthe turn was not sent\x1b[0m")
		return false
	}

	fmt.Fprintf(os.Stderr, "\x1b[2m⏎ send it anyway · anything else leaves it here\x1b[0m\n\x1b[1m› \x1b[0m")
	line, readErr := in.ReadString('\n')
	if readErr != nil || strings.TrimSpace(line) != "" {
		fmt.Fprintln(os.Stderr, "\x1b[2mthe turn was not sent · /limit raises the ceiling, /compact makes room\x1b[0m")
		return false
	}
	return true
}

func workdir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func addUsage(total *llm.Usage, u llm.Usage) {
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.ThinkingTokens += u.ThinkingTokens
	total.CacheReadTokens += u.CacheReadTokens
	total.CacheWriteTokens += u.CacheWriteTokens
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
