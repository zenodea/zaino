package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/zenodea/zaino/internal/httpx"
	"github.com/zenodea/zaino/internal/llm"
)

const (
	defaultBaseURL   = "https://generativelanguage.googleapis.com/v1beta"
	defaultMaxTokens = 8192
)

var ErrNoCredentials = errors.New("gemini: no credentials: set GEMINI_API_KEY " +
	"(or GOOGLE_API_KEY), or pass WithAPIKey")

type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *httpx.Client
}

type Option func(*Client)

func WithAPIKey(key string) Option  { return func(c *Client) { c.apiKey = key } }
func WithBaseURL(url string) Option { return func(c *Client) { c.baseURL = url } }

func WithDefaultModel(model string) Option { return func(c *Client) { c.model = model } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http.HTTP = h } }

func WithMaxRetries(n int) Option { return func(c *Client) { c.http.MaxRetries = n } }

func New(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL: defaultBaseURL,
		model:   Model25Pro,
		http: &httpx.Client{
			HTTP:       httpx.NewClient(),
			MaxRetries: 2,
			ParseError: func(status int, header http.Header, body []byte) error {
				e := parseAPIError(status, body)
				e.retryAfter = httpx.ParseRetryAfter(header)
				return e
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.apiKey == "" {
		c.apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if c.apiKey == "" {
		c.apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if c.apiKey == "" {
		return nil, ErrNoCredentials
	}
	if base := os.Getenv("GEMINI_BASE_URL"); base != "" && c.baseURL == defaultBaseURL {
		c.baseURL = base
	}
	return c, nil
}

func (c *Client) setRequestHeaders(req *http.Request) {
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("accept", "text/event-stream")
}

func (c *Client) Name() string { return "gemini" }

func (c *Client) DefaultModel() string { return c.model }

func (c *Client) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	wire, err := buildRequest(req, defaultMaxTokens)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("gemini: encoding request: %w", err)
	}

	model := req.Model
	if model == "" {
		model = c.DefaultModel()
	}
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse",
		c.baseURL, url.PathEscape(model))

	resp, err := c.http.Post(ctx, endpoint, c.setRequestHeaders, body)
	if err != nil {
		return nil, err
	}
	return newStream(resp.Body), nil
}
