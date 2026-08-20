package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/x/httpx"
)

// Config describes one OpenAI-compatible host. Everything that differs
// between OpenAI, xAI and the rest lives here rather than in the client.
type Config struct {
	Name         string
	BaseURL      string
	DefaultModel string
	KnownModels  []string

	// Checked in order; the first one set wins.
	APIKeyEnv  []string
	BaseURLEnv string

	// Hosts that predate max_completion_tokens.
	LegacyMaxTokens bool

	// Whether reasoning_effort is understood.
	ReasoningEffort bool

	// Newer OpenAI models want "developer" where everyone else wants "system".
	DeveloperRole bool
}

func (c Config) systemRole() string {
	if c.DeveloperRole {
		return "developer"
	}
	return "system"
}

var ErrNoCredentials = errors.New("no credentials")

// Wraps ErrNoCredentials so callers can test for it, while reading as one
// sentence that names the host and the variables it looks at.
type missingCredentials struct{ cfg Config }

func (e *missingCredentials) Unwrap() error { return ErrNoCredentials }

func (e *missingCredentials) Error() string {
	return fmt.Sprintf("%s: no credentials: set %s, or pass WithAPIKey",
		e.cfg.Name, strings.Join(e.cfg.APIKeyEnv, " or "))
}

type Client struct {
	cfg     Config
	apiKey  string
	baseURL string
	model   string
	http    *httpx.Client

	pricesMu sync.Mutex
	prices   map[string]llm.Price
}

type Option func(*Client)

func WithAPIKey(key string) Option  { return func(c *Client) { c.apiKey = key } }
func WithBaseURL(url string) Option { return func(c *Client) { c.baseURL = url } }

func WithDefaultModel(model string) Option { return func(c *Client) { c.model = model } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http.HTTP = h } }

func WithMaxRetries(n int) Option { return func(c *Client) { c.http.MaxRetries = n } }

func New(cfg Config, opts ...Option) (*Client, error) {
	c := &Client{
		cfg:     cfg,
		baseURL: cfg.BaseURL,
		model:   cfg.DefaultModel,
		http: &httpx.Client{
			HTTP:       httpx.NewClient(),
			MaxRetries: 2,
			ParseError: func(status int, header http.Header, body []byte) error {
				e := parseAPIError(cfg.Name, status, header.Get("x-request-id"), body)
				e.retryAfter = httpx.ParseRetryAfter(header)
				return e
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	for _, env := range cfg.APIKeyEnv {
		if c.apiKey != "" {
			break
		}
		c.apiKey = os.Getenv(env)
	}
	if c.apiKey == "" {
		return nil, &missingCredentials{cfg: cfg}
	}
	if url := os.Getenv(cfg.BaseURLEnv); url != "" && c.baseURL == cfg.BaseURL {
		c.baseURL = url
	}
	return c, nil
}

func (c *Client) Name() string { return c.cfg.Name }

func (c *Client) DefaultModel() string { return c.model }

func (c *Client) Models() []string {
	return append([]string(nil), c.cfg.KnownModels...)
}

func (c *Client) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	wire, err := buildRequest(req, c.cfg, c.DefaultModel())
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("%s: encoding request: %w", c.cfg.Name, err)
	}

	resp, err := c.http.Post(ctx, c.baseURL+"/chat/completions", c.setRequestHeaders, body)
	if err != nil {
		return nil, err
	}
	return newStream(c.cfg.Name, resp.Body), nil
}

func (c *Client) setRequestHeaders(req *http.Request) {
	req.Header.Set("authorization", "Bearer "+c.apiKey)
	req.Header.Set("accept", "text/event-stream")
}
