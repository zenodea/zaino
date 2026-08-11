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

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/wirelog"
)

type Options struct {
	Provider     string
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
		usage.InputTokens += u.InputTokens
		usage.OutputTokens += u.OutputTokens
		usage.ThinkingTokens += u.ThinkingTokens
		usage.CacheReadTokens += u.CacheReadTokens
		usage.CacheWriteTokens += u.CacheWriteTokens

		o.Recorder.Turn(u)
		o.Wire.Turn(resp.Model, resp.StopReason, u)

		if o.Verbose {
			fmt.Fprintf(os.Stderr,
				"\n\x1b[2m[%s in=%d out=%d think=%d cache_read=%d stop=%s]\x1b[0m\n",
				resp.Model, u.InputTokens, u.OutputTokens, u.ThinkingTokens,
				u.CacheReadTokens, resp.StopReason)
		}
	}

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
			messages = runTurn(ag, interrupts, messages, strings.TrimSpace(line), stdout, o.Recorder)
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
			cleared, quit := runCommand(ag, prompt, messages, &usage, o)
			if quit {
				return nil
			}
			if cleared {
				messages = nil
			}
			continue
		}
		messages = runTurn(ag, interrupts, messages, prompt, stdout, o.Recorder)
	}
}

func runTurn(ag *agent.Agent, interrupts *interrupts, messages []llm.Message,
	prompt string, stdout *bufio.Writer, rec *session.Recorder) []llm.Message {
	messages = append(messages, llm.UserText(prompt))

	ctx, cancel := context.WithCancel(context.Background())
	interrupts.bind(cancel)
	messages, err := ag.Run(ctx, messages)
	interrupts.unbind()
	cancel()

	if saveErr := rec.Messages(messages); saveErr != nil {
		fmt.Fprintln(os.Stderr, "\x1b[31msession not being saved: "+saveErr.Error()+"\x1b[0m")
	}

	stdout.Flush()
	fmt.Fprintln(os.Stderr)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "\x1b[2minterrupted\x1b[0m")
		} else {
			fmt.Fprintln(os.Stderr, "\x1b[31m"+err.Error()+"\x1b[0m")
		}
	}
	return messages
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
