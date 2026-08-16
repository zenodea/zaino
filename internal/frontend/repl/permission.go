package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zenodea/zaino/internal/store/session"

	"github.com/zenodea/zaino/internal/permission"
)

type approver struct {
	in  *bufio.Reader
	rec *session.Recorder
}

func (a *approver) Approve(ctx context.Context, req permission.Request) (permission.Grant, error) {
	grant, err := a.ask(ctx, req)
	if err == nil {
		a.rec.Append(session.Permission(req.Tool, string(req.Action), req.Target, decisionName(grant)))
	}
	return grant, err
}

func decisionName(grant permission.Grant) string {
	switch grant {
	case permission.Once:
		return "allowed"
	case permission.Always:
		return "allowed-always"
	}
	return "refused"
}

func (a *approver) ask(ctx context.Context, req permission.Request) (permission.Grant, error) {
	fmt.Fprintf(os.Stderr, "\n\x1b[1m%s %s\x1b[0m\n", verb(req.Action), req.Target)
	if req.Preview != "" {
		fmt.Fprintf(os.Stderr, "\x1b[2m%s\x1b[0m\n", indent(req.Preview))
	}

	for {
		if err := ctx.Err(); err != nil {
			return permission.Reject, err
		}
		fmt.Fprint(os.Stderr, "\x1b[2m[y] allow  [a] allow for the session  [n] refuse\x1b[0m › ")

		line, err := a.in.ReadString('\n')
		// Enter means yes, but only when someone pressed it: a closed stdin
		// hands back the same empty line and must never approve.
		if err != nil && strings.TrimSpace(line) == "" {
			fmt.Fprintln(os.Stderr)
			return permission.Reject, nil
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "":
			return permission.Once, nil
		case "a", "always":
			return permission.Always, nil
		case "n", "no":
			return permission.Reject, nil
		}
		if err != nil {
			fmt.Fprintln(os.Stderr)
			return permission.Reject, nil
		}
		fmt.Fprintln(os.Stderr, "\x1b[2mAnswer y, a or n.\x1b[0m")
	}
}

func verb(action permission.Action) string {
	switch action {
	case permission.Write:
		return "Write"
	case permission.Execute:
		return "Run"
	case permission.Network:
		return "Fetch"
	}
	return "Read"
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 20 {
		hidden := len(lines) - 20
		lines = append(lines[:20], fmt.Sprintf("… %d more lines", hidden))
	}
	return "  " + strings.Join(lines, "\n  ")
}
