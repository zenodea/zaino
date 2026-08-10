package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/zenodea/zaino/internal/httpx"
	"github.com/zenodea/zaino/internal/llm"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"

	oauthBeta = "oauth-2025-04-20"
)

var ErrNoCredentials = errors.New("anthropic: no credentials: set ANTHROPIC_API_KEY, " +
	"set ANTHROPIC_AUTH_TOKEN, or pass WithAPIKey/WithAuthToken")

type Client struct {
	apiKey    string
	authToken string
	baseURL   string
	betas     []string
	http      *httpx.Client
}

type Option func(*Client)

func WithAPIKey(key string) Option    { return func(c *Client) { c.apiKey = key } }
func WithAuthToken(tok string) Option { return func(c *Client) { c.authToken = tok } }
func WithBaseURL(url string) Option   { return func(c *Client) { c.baseURL = url } }

func WithBetas(betas ...string) Option { return func(c *Client) { c.betas = betas } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http.HTTP = h } }

func WithMaxRetries(n int) Option { return func(c *Client) { c.http.MaxRetries = n } }

func New(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL: defaultBaseURL,
		http: &httpx.Client{
			HTTP:       httpx.NewClient(),
			MaxRetries: 2,
			ParseError: func(status int, header http.Header, body []byte) error {
				e := parseAPIError(status, header.Get("request-id"), body)
				e.retryAfter = httpx.ParseRetryAfter(header)
				return e
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.apiKey == "" && c.authToken == "" {
		c.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if c.apiKey == "" && c.authToken == "" {
		c.authToken = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	if c.apiKey == "" && c.authToken == "" {
		return nil, ErrNoCredentials
	}
	if url := os.Getenv("ANTHROPIC_BASE_URL"); url != "" && c.baseURL == defaultBaseURL {
		c.baseURL = url
	}
	return c, nil
}

func (c *Client) Name() string { return "anthropic" }

func (c *Client) DefaultModel() string { return ModelOpus5 }

func (c *Client) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	body, err := json.Marshal(buildRequest(req, c.DefaultModel()))
	if err != nil {
		return nil, fmt.Errorf("anthropic: encoding request: %w", err)
	}

	resp, err := c.http.Post(ctx, c.baseURL+"/v1/messages", c.setRequestHeaders, body)
	if err != nil {
		return nil, err
	}
	return newStream(resp.Body), nil
}

func (c *Client) setRequestHeaders(req *http.Request) {
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("anthropic-version", apiVersion)

	betas := c.betas
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	} else {
		req.Header.Set("authorization", "Bearer "+c.authToken)
		betas = append(append([]string(nil), betas...), oauthBeta)
	}
	for _, beta := range betas {
		req.Header.Add("anthropic-beta", beta)
	}
}
