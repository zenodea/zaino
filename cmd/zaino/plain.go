package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
)

func runPlain(ag *agent.Agent, providerName string, showThinking, verbose bool) error {
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
	if showThinking {
		ag.Hooks.OnThinkingDelta = func(text string) {
			fmt.Fprintf(os.Stderr, "\x1b[2m%s\x1b[0m", text)
		}
	}
	if verbose {
		ag.Hooks.OnTurn = func(resp *llm.Response) {
			u := resp.Usage
			fmt.Fprintf(os.Stderr,
				"\n\x1b[2m[%s in=%d out=%d think=%d cache_read=%d stop=%s]\x1b[0m\n",
				resp.Model, u.InputTokens, u.OutputTokens, u.ThinkingTokens,
				u.CacheReadTokens, resp.StopReason)
		}
	}

	interrupts := newInterrupts()
	defer interrupts.stop()

	var history []llm.Message
	in := bufio.NewReader(os.Stdin)

	for {
		fmt.Fprintf(os.Stderr, "\x1b[1m› \x1b[0m")
		line, err := in.ReadString('\n')
		if errors.Is(err, io.EOF) {
			if strings.TrimSpace(line) == "" {
				fmt.Fprintln(os.Stderr)
				return nil
			}

			defer fmt.Fprintln(os.Stderr)
			history = runTurn(ag, interrupts, history, strings.TrimSpace(line), stdout)
			return nil
		}
		if err != nil {
			return err
		}

		prompt := strings.TrimSpace(line)
		switch prompt {
		case "":
			continue
		case "/exit", "/quit":
			return nil
		case "/reset":
			history = nil
			fmt.Fprintln(os.Stderr, "\x1b[2mcontext cleared\x1b[0m")
			continue
		}
		history = runTurn(ag, interrupts, history, prompt, stdout)
	}
}

func runTurn(ag *agent.Agent, interrupts *interrupts, history []llm.Message,
	prompt string, stdout *bufio.Writer) []llm.Message {
	history = append(history, llm.UserText(prompt))

	ctx, cancel := context.WithCancel(context.Background())
	interrupts.bind(cancel)
	history, err := ag.Run(ctx, history)
	interrupts.unbind()
	cancel()

	stdout.Flush()
	fmt.Fprintln(os.Stderr)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "\x1b[2minterrupted\x1b[0m")
		} else {
			fmt.Fprintln(os.Stderr, "\x1b[31m"+err.Error()+"\x1b[0m")
		}
	}
	return history
}

type interrupts struct {
	ch chan os.Signal

	mu     sync.Mutex
	cancel context.CancelFunc
}

func newInterrupts() *interrupts {
	i := &interrupts{ch: make(chan os.Signal, 1)}
	signal.Notify(i.ch, os.Interrupt)
	go func() {
		for range i.ch {
			i.mu.Lock()
			cancel := i.cancel
			i.mu.Unlock()
			if cancel == nil {
				fmt.Fprintln(os.Stderr)
				os.Exit(130)
			}
			cancel()
		}
	}()
	return i
}

func (i *interrupts) bind(cancel context.CancelFunc) {
	i.mu.Lock()
	i.cancel = cancel
	i.mu.Unlock()
}

func (i *interrupts) unbind() {
	i.mu.Lock()
	i.cancel = nil
	i.mu.Unlock()
}

func (i *interrupts) stop() {
	signal.Stop(i.ch)
	close(i.ch)
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
