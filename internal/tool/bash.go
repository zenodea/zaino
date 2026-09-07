package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Bash struct{ w *Workspace }

type bashArgs struct {
	Command    string `json:"command"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
	Background bool   `json:"background,omitempty"`
}

const (
	defaultTimeout = 2 * time.Minute
	maxTimeout     = 10 * time.Minute
)

func (b *Bash) Action() permission.Action { return permission.Execute }

func (b *Bash) Definition() llm.Tool {
	return llm.Tool{
		Name: "bash",
		Description: "Run a shell command in the working directory and return its output. " +
			"Prefer read, find and grep for looking at files: they are cheaper and do not " +
			"need approval. Each call starts in the working directory, so a cd does not carry over.",
		InputSchema: object(map[string]any{
			"command":    field("string", "Command line to run, interpreted by sh."),
			"timeout_ms": field("integer", fmt.Sprintf("How long to allow, in milliseconds. Defaults to %d.", defaultTimeout.Milliseconds())),
			"background": field("boolean", "Start it and return at once, for servers, watchers and anything long. "+
				"The result names a job the job tool reads or stops; it is stopped when zaino exits."),
		}, "command"),
	}
}

func (b *Bash) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[bashArgs](input)
	if err != nil {
		return nil, err
	}
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := defaultTimeout
	if args.TimeoutMS > 0 {
		timeout = min(time.Duration(args.TimeoutMS)*time.Millisecond, maxTimeout)
	}
	return &bashCall{dir: b.w.Root, command: command, timeout: timeout, background: args.Background}, nil
}

type reporter func(string)

func (r reporter) Write(p []byte) (int, error) {
	r(string(p))
	return len(p), nil
}

type bashCall struct {
	dir        string
	command    string
	timeout    time.Duration
	background bool
}

func (c *bashCall) Request() permission.Request {
	preview := c.command
	if c.background {
		preview += "   (in the background)"
	}
	return permission.Request{
		Tool:    "bash",
		Action:  permission.Execute,
		Target:  c.command,
		Preview: preview,
	}
}

func (c *bashCall) Run(ctx context.Context) (string, error) {
	if c.background {
		j, err := jobs.start(c.dir, c.command)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("started %s (pid %d) in the background. job {\"id\": %q} reads its output so far, "+
			"job {\"id\": %q, \"action\": \"kill\"} stops it.", j.id, j.cmd.Process.Pid, j.id, j.id), nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", c.command)
	cmd.Dir = c.dir

	// One writer for both streams, so exec gives them one pipe and the
	// interleaving is the terminal's; the listener, if any, sees each chunk
	// as it lands rather than everything at the end.
	var out bytes.Buffer
	var sink io.Writer = &out
	if report := Progress(ctx); report != nil {
		sink = io.MultiWriter(&out, reporter(report))
	}
	cmd.Stdout, cmd.Stderr = sink, sink
	err := cmd.Run()
	text := clipOutput(strings.TrimRight(out.String(), "\n"))

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("timed out after %s\n%s", c.timeout, text)
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if text == "" {
				return "", fmt.Errorf("exit status %d", exit.ExitCode())
			}
			return "", fmt.Errorf("exit status %d\n%s", exit.ExitCode(), text)
		}
		return "", err
	}
	if text == "" {
		return "(no output)", nil
	}
	return text, nil
}
