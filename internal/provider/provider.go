package provider

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider/anthropic"
	"github.com/zenodea/zaino/internal/provider/gemini"
)

// Option adjusts how a provider is built. The set is deliberately small: it
// exists so the wire log can sit under both clients.
type Option func(*config)

type config struct {
	http *http.Client
}

// WithHTTPClient hands the provider the client to send with.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) { c.http = h }
}

var constructors = map[string]func(config) (llm.Provider, error){
	"anthropic": newAnthropic,
	"claude":    newAnthropic,
	"gemini":    newGemini,
	"google":    newGemini,
}

func newAnthropic(c config) (llm.Provider, error) {
	var opts []anthropic.Option
	if c.http != nil {
		opts = append(opts, anthropic.WithHTTPClient(c.http))
	}
	return anthropic.New(opts...)
}

func newGemini(c config) (llm.Provider, error) {
	var opts []gemini.Option
	if c.http != nil {
		opts = append(opts, gemini.WithHTTPClient(c.http))
	}
	return gemini.New(opts...)
}

var autoOrder = []string{"anthropic", "gemini"}

var ErrNoCredentials = errors.New("provider: no credentials for any provider")

func New(name string, opts ...Option) (llm.Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	if name == "" || name == "auto" {
		var missing []error
		for _, candidate := range autoOrder {
			p, err := constructors[candidate](cfg)
			if err == nil {
				return p, nil
			}
			missing = append(missing, err)
		}
		return nil, fmt.Errorf("%w:\n  %s", ErrNoCredentials, joinErrors(missing))
	}

	build, ok := constructors[name]
	if !ok {
		return nil, fmt.Errorf("provider: unknown provider %q (available: %s)",
			name, strings.Join(Available(), ", "))
	}
	return build(cfg)
}

func Available() []string {
	names := append([]string(nil), autoOrder...)
	sort.Strings(names)
	return names
}

func joinErrors(errs []error) string {
	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n  ")
}
