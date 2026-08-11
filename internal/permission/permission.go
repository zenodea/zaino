package permission

import (
	"context"
	"fmt"
)

type Action string

const (
	Read    Action = "read"
	Write   Action = "write"
	Execute Action = "execute"
)

type Request struct {
	Tool    string
	Action  Action
	Target  string
	Preview string

	Outside bool
}

type Grant int

const (
	Reject Grant = iota
	Once
	Always
)

type Approver interface {
	Approve(ctx context.Context, req Request) (Grant, error)
}

type DeniedError struct {
	Request Request
	Reason  string
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("%s denied: %s", e.Request.Tool, e.Reason)
}

func Denied(err error) bool {
	_, ok := err.(*DeniedError)
	return ok
}
