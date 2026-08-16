package llm

import (
	"context"
	"encoding/json"
)

type Provider interface {
	Name() string

	DefaultModel() string

	Stream(ctx context.Context, req Request) (Stream, error)
}

// ModelLister is optional: a provider that knows its own model ids says so.
type ModelLister interface {
	Models() []string
}

// ModelFetcher is optional: a provider that can ask its host which models the
// credential actually reaches. Line-ups move faster than releases, so a list
// compiled here goes stale; this one cannot.
type ModelFetcher interface {
	FetchModels(ctx context.Context) ([]string, error)
}

// TokenCounter is optional: a provider that will say, before a request is
// sent, exactly how much of the window it occupies. A ceiling measured with
// the estimator is a ceiling in roughly the right place.
type TokenCounter interface {
	CountTokens(ctx context.Context, req Request) (int, error)
}

type Stream interface {
	Next() bool
	Event() Event
	Err() error

	Message() *Response
	Close() error
}

type Event interface{ isEvent() }

type MessageStartEvent struct{ Message Response }

type ContentBlockStartEvent struct {
	Index int
	Block ContentBlock
}

type ContentBlockDeltaEvent struct {
	Index int
	Delta Delta
}

type ContentBlockStopEvent struct{ Index int }

type MessageDeltaEvent struct {
	StopReason  StopReason
	StopDetails *StopDetails
	Usage       Usage
}

type MessageStopEvent struct{}

type PingEvent struct{}

func (MessageStartEvent) isEvent()      {}
func (ContentBlockStartEvent) isEvent() {}
func (ContentBlockDeltaEvent) isEvent() {}
func (ContentBlockStopEvent) isEvent()  {}
func (MessageDeltaEvent) isEvent()      {}
func (MessageStopEvent) isEvent()       {}
func (PingEvent) isEvent()              {}

type Delta interface{ isDelta() }

type TextDelta struct{ Text string }
type ThinkingDelta struct{ Thinking string }
type SignatureDelta struct{ Signature string }

type InputJSONDelta struct{ PartialJSON string }

type OpaqueDelta struct {
	Type string
	Raw  json.RawMessage
}

func (TextDelta) isDelta()      {}
func (ThinkingDelta) isDelta()  {}
func (SignatureDelta) isDelta() {}
func (InputJSONDelta) isDelta() {}
func (OpaqueDelta) isDelta()    {}
