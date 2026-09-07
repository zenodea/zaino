package tool

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type Change struct {
	Path    string
	Existed bool
	Before  []byte
	After   []byte
	Mode    fs.FileMode
}

// Changed is optional: a call that wrote files says which, once it has run.
type Changed interface {
	Changes() []Change
}

type rootCallKey struct{}

// WithRootCallID names the outermost call a change belongs to, so a
// subagent's edits land on the task call that spawned it.
func WithRootCallID(ctx context.Context, id string) context.Context {
	if RootCallID(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, rootCallKey{}, id)
}

func RootCallID(ctx context.Context) string {
	id, _ := ctx.Value(rootCallKey{}).(string)
	return id
}

func refuseGit(p Path) error {
	for _, seg := range strings.Split(filepath.ToSlash(p.Rel), "/") {
		if seg == ".git" {
			return fmt.Errorf("%s is inside .git — not touching the repository's own files", p)
		}
	}
	return nil
}
