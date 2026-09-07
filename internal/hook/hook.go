package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

type Event string

const (
	SessionStart Event = "session-start"
	PreTool      Event = "pre-tool"
	PostTool     Event = "post-tool"
	TurnEnd      Event = "turn-end"
)

var Events = []Event{SessionStart, PreTool, PostTool, TurnEnd}

const DefaultTimeout = 30 * time.Second

type Hook struct {
	Event   Event
	Tools   []string
	Run     string
	Timeout time.Duration
}

func (h Hook) matches(p Payload) bool {
	if h.Event != p.Event {
		return false
	}
	return len(h.Tools) == 0 || p.Tool == "" || slices.Contains(h.Tools, p.Tool)
}

type Payload struct {
	Event   Event           `json:"event"`
	Tool    string          `json:"tool,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"error,omitempty"`
	Cwd     string          `json:"cwd"`
	Session string          `json:"session,omitempty"`
}

type Outcome struct {
	Blocked string
	Said    []string
	Failed  []error
}

type Blocked struct {
	Run    string
	Reason string
}

func (e *Blocked) Error() string {
	return "blocked by a hook: " + e.Reason
}

type Set struct {
	Dir     string
	Session string
	hooks   []Hook
}

func New(dir string, hooks []Hook) *Set {
	return &Set{Dir: dir, hooks: hooks}
}

func (s *Set) List() []Hook {
	if s == nil {
		return nil
	}
	return append([]Hook(nil), s.hooks...)
}

func (s *Set) Has(event Event) bool {
	if s == nil {
		return false
	}
	for _, h := range s.hooks {
		if h.Event == event {
			return true
		}
	}
	return false
}

func (s *Set) Fire(ctx context.Context, p Payload) Outcome {
	var out Outcome
	if s == nil {
		return out
	}
	p.Cwd, p.Session = s.Dir, s.Session
	for _, h := range s.hooks {
		if !h.matches(p) {
			continue
		}
		stdout, stderr, err := s.exec(ctx, h, p)
		var exit *exec.ExitError
		switch {
		case err == nil:
			if stdout != "" {
				out.Said = append(out.Said, stdout)
			}
		case errors.As(err, &exit) && exit.ExitCode() == 2:
			reason := stderr
			if reason == "" {
				reason = stdout
			}
			if reason == "" {
				reason = "hook " + h.Run + " said no"
			}
			out.Blocked = reason
			return out
		default:
			msg := err.Error()
			if stderr != "" {
				msg += ": " + stderr
			}
			out.Failed = append(out.Failed, fmt.Errorf("hook %q: %s", h.Run, msg))
		}
	}
	return out
}

func (s *Set) exec(ctx context.Context, h Hook, p Payload) (stdout, stderr string, err error) {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(p)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", h.Run)
	cmd.Dir = s.Dir
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = append(os.Environ(),
		"ZAINO_EVENT="+string(p.Event), "ZAINO_TOOL="+p.Tool, "ZAINO_SESSION="+p.Session)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", timeout)
	}
	return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), err
}
