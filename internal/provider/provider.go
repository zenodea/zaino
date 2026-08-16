package provider

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider/anthropic"
	"github.com/zenodea/zaino/internal/provider/gemini"
	"github.com/zenodea/zaino/internal/provider/grok"
	"github.com/zenodea/zaino/internal/provider/openai"
	"github.com/zenodea/zaino/internal/provider/openaicompat"
	"github.com/zenodea/zaino/internal/provider/openrouter"
	"github.com/zenodea/zaino/internal/store/credentials"
	"github.com/zenodea/zaino/internal/x/antcli"
)

// Option adjusts how a provider is built. The set is deliberately small: it
// exists so the wire log can sit under both clients.
type Option func(*config)

type config struct {
	http  *http.Client
	store *credentials.Store
}

// A stored credential is the fallback for someone who never exported one, so
// it is only consulted after the environment has been given its turn.
func (c config) stored(name string) (credentials.Entry, bool) {
	if c.store == nil {
		return credentials.Entry{}, false
	}
	entry, ok, err := c.store.Lookup(name)
	if err != nil || !ok {
		return credentials.Entry{}, false
	}
	return entry, true
}

// WithHTTPClient hands the provider the client to send with.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) { c.http = h }
}

// WithCredentials consults a store when the environment holds nothing.
func WithCredentials(s *credentials.Store) Option {
	return func(c *config) { c.store = s }
}

var (
	storeMu      sync.RWMutex
	defaultStore *credentials.Store
)

// SetStore installs the store every New consults by default. Frontends call
// New with no options, so the process sets this once at startup.
func SetStore(s *credentials.Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	defaultStore = s
}

func currentStore() *credentials.Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return defaultStore
}

var constructors = map[string]func(config) (llm.Provider, error){
	"anthropic":  newAnthropic,
	"claude":     newAnthropic,
	"gemini":     newGemini,
	"google":     newGemini,
	"openai":     newOpenAI,
	"gpt":        newOpenAI,
	"grok":       newGrok,
	"xai":        newGrok,
	"openrouter": newOpenRouter,
	"router":     newOpenRouter,
}

func newAnthropic(c config) (llm.Provider, error) {
	var opts []anthropic.Option
	if c.http != nil {
		opts = append(opts, anthropic.WithHTTPClient(c.http))
	}
	if p, err := anthropic.New(opts...); err == nil {
		return p, nil
	} else if entry, ok := c.stored("anthropic"); ok {
		switch entry.Method {
		case credentials.OAuth:
			cli := &antcli.CLI{Profile: entry.Profile}
			return anthropic.New(append(opts, anthropic.WithTokenSource(cli.AccessToken))...)
		case credentials.APIKey:
			return anthropic.New(append(opts, anthropic.WithAPIKey(entry.Key))...)
		}
		return nil, err
	} else {
		return nil, err
	}
}

func newGemini(c config) (llm.Provider, error) {
	var opts []gemini.Option
	if c.http != nil {
		opts = append(opts, gemini.WithHTTPClient(c.http))
	}
	p, err := gemini.New(opts...)
	if err == nil {
		return p, nil
	}
	if entry, ok := c.stored("gemini"); ok && entry.Key != "" {
		return gemini.New(append(opts, gemini.WithAPIKey(entry.Key))...)
	}
	return nil, err
}

func newOpenAI(c config) (llm.Provider, error) { return newCompat(c, "openai", openai.New) }

func newGrok(c config) (llm.Provider, error) { return newCompat(c, "grok", grok.New) }

func newOpenRouter(c config) (llm.Provider, error) {
	return newCompat(c, "openrouter", openrouter.New)
}

func newCompat(c config, name string,
	build func(...openaicompat.Option) (*openaicompat.Client, error)) (llm.Provider, error) {

	var opts []openaicompat.Option
	if c.http != nil {
		opts = append(opts, openaicompat.WithHTTPClient(c.http))
	}
	p, err := build(opts...)
	if err == nil {
		return p, nil
	}
	// Neither OpenAI nor xAI offers OAuth for API access, so a stored
	// credential here is always a key.
	if entry, ok := c.stored(name); ok && entry.Key != "" {
		return build(append(opts, openaicompat.WithAPIKey(entry.Key))...)
	}
	return nil, err
}

var autoOrder = []string{"anthropic", "gemini", "openai", "grok", "openrouter"}

var ErrNoCredentials = errors.New("provider: no credentials for any provider")

func New(name string, opts ...Option) (llm.Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	cfg := config{store: currentStore()}
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

// HasCredentials reports whether a provider could be built right now. It
// constructs and discards one, which touches no network.
func HasCredentials(name string) bool {
	_, err := New(name)
	return err == nil
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
