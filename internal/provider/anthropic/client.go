package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/x/httpx"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"

	oauthBeta = "oauth-2025-04-20"
)

var ErrNoCredentials = errors.New("anthropic: no credentials: set ANTHROPIC_API_KEY, " +
	"set ANTHROPIC_AUTH_TOKEN, log in with `ant auth login`, or pass WithAPIKey/WithAuthToken")

type Client struct {
	apiKey    string
	authToken string
	baseURL   string
	betas     []string
	http      *httpx.Client

	// Set when the token is minted per request rather than held. OAuth
	// tokens are short-lived, so one captured at startup would go stale
	// partway through a long session.
	tokenSource func(context.Context) (string, error)
}

type Option func(*Client)

func WithAPIKey(key string) Option    { return func(c *Client) { c.apiKey = key } }
func WithAuthToken(tok string) Option { return func(c *Client) { c.authToken = tok } }
func WithBaseURL(url string) Option   { return func(c *Client) { c.baseURL = url } }

func WithBetas(betas ...string) Option { return func(c *Client) { c.betas = betas } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http.HTTP = h } }

// WithTokenSource authenticates with a bearer token fetched just before each
// request, which is how OAuth credentials have to be handled.
func WithTokenSource(fn func(context.Context) (string, error)) Option {
	return func(c *Client) { c.tokenSource = fn }
}

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

	if c.tokenSource == nil {
		if c.apiKey == "" && c.authToken == "" {
			c.apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if c.apiKey == "" && c.authToken == "" {
			c.authToken = os.Getenv("ANTHROPIC_AUTH_TOKEN")
		}
		if c.apiKey == "" && c.authToken == "" {
			return nil, ErrNoCredentials
		}
	}
	if url := os.Getenv("ANTHROPIC_BASE_URL"); url != "" && c.baseURL == defaultBaseURL {
		c.baseURL = url
	}
	return c, nil
}

func (c *Client) Name() string { return "anthropic" }

func (c *Client) DefaultModel() string { return ModelOpus5 }

func (c *Client) Models() []string {
	return []string{
		ModelFable5,
		ModelOpus5, ModelOpus48, ModelOpus47, ModelOpus46,
		ModelSonnet5, ModelSonnet46,
		ModelHaiku45,
	}
}

func (c *Client) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	body, err := json.Marshal(buildRequest(req, c.DefaultModel()))
	if err != nil {
		return nil, fmt.Errorf("anthropic: encoding request: %w", err)
	}

	key, token, err := c.credential(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Post(ctx, c.baseURL+"/v1/messages", c.setRequestHeaders(key, token), body)
	if err != nil {
		return nil, err
	}
	return newStream(resp.Body), nil
}

// CountTokens asks what the request would occupy before it is sent, which is
// the only measure a hard context limit can honestly be enforced against.
func (c *Client) CountTokens(ctx context.Context, req llm.Request) (int, error) {
	body, err := json.Marshal(buildCountRequest(req, c.DefaultModel()))
	if err != nil {
		return 0, fmt.Errorf("anthropic: encoding count request: %w", err)
	}

	key, token, err := c.credential(ctx)
	if err != nil {
		return 0, err
	}

	setHeaders := c.setRequestHeaders(key, token)
	resp, err := c.http.Post(ctx, c.baseURL+"/v1/messages/count_tokens", func(r *http.Request) {
		setHeaders(r)
		r.Header.Set("accept", "application/json")
	}, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("anthropic: decoding token count: %w", err)
	}
	return out.InputTokens, nil
}

func (c *Client) credential(ctx context.Context) (key, token string, err error) {
	if c.tokenSource == nil {
		return c.apiKey, c.authToken, nil
	}
	token, err = c.tokenSource(ctx)
	if err != nil {
		return "", "", fmt.Errorf("anthropic: %w", err)
	}
	if token == "" {
		return "", "", ErrNoCredentials
	}
	return "", token, nil
}

// Sending both an API key and a bearer token has the API reject the request,
// so exactly one of them goes out.
func (c *Client) setRequestHeaders(key, token string) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set("accept", "text/event-stream")
		req.Header.Set("anthropic-version", apiVersion)

		betas := c.betas
		if key != "" {
			req.Header.Set("x-api-key", key)
		} else {
			req.Header.Set("authorization", "Bearer "+token)
			// /v1/messages rejects an OAuth token without this.
			betas = append(append([]string(nil), betas...), oauthBeta)
		}
		for _, beta := range betas {
			req.Header.Add("anthropic-beta", beta)
		}
	}
}
