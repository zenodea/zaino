package provider

import (
	"context"
	"errors"

	"github.com/zenodea/zaino/internal/llm"
)

// NoneName is what an unconfigured provider calls itself. It is not a real
// provider name, so it never collides with one.
const NoneName = "none"

var ErrNotConfigured = errors.New("no provider is set up yet — /provider to pick one")

// none stands in when nothing is configured. Startup would otherwise die
// before the UI exists, which is the worst moment to explain a missing key.
type none struct{ reason error }

// None returns a provider that carries the reason it cannot send anything.
func None(reason error) llm.Provider {
	if reason == nil {
		reason = ErrNotConfigured
	}
	return none{reason: reason}
}

func (none) Name() string { return NoneName }

func (none) DefaultModel() string { return "" }

func (none) Models() []string { return nil }

func (n none) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return nil, n.reason
}

// IsNone reports whether a provider is the unconfigured placeholder.
func IsNone(p llm.Provider) bool {
	_, ok := p.(none)
	return ok
}

// Reason recovers what stopped a provider being built, for a frontend that
// wants to show it.
func Reason(p llm.Provider) error {
	if n, ok := p.(none); ok {
		return n.reason
	}
	return nil
}
