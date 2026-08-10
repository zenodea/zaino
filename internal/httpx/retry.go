package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

const MaxErrorBody = 1 << 20

type Retryable interface {
	error
	Retryable() bool
}

type RetryAfter interface {
	RetryAfter() time.Duration
}

// No overall timeout: a streaming turn can run for minutes. Bound it with
// the request context instead.
func NewTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 5 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
	}
}

func NewClient() *http.Client {
	return &http.Client{Transport: NewTransport()}
}

type Client struct {
	HTTP       *http.Client
	MaxRetries int

	ParseError func(status int, header http.Header, body []byte) error
}

func (c *Client) Post(ctx context.Context, url string, setHeaders func(*http.Request), body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		resp, err := c.attempt(ctx, url, setHeaders, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if attempt >= c.MaxRetries || !shouldRetry(err) || ctx.Err() != nil {
			return nil, lastErr
		}

		delay := Backoff(attempt)
		var after RetryAfter
		if errors.As(err, &after) && after.RetryAfter() > 0 {
			delay = after.RetryAfter()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (c *Client) attempt(ctx context.Context, url string, setHeaders func(*http.Request), body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	if setHeaders != nil {
		setHeaders(req)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}

	defer resp.Body.Close()
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBody))
	return nil, c.ParseError(resp.StatusCode, resp.Header, errBody)
}

func shouldRetry(err error) bool {
	var r Retryable
	if errors.As(err, &r) {
		return r.Retryable()
	}

	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func Backoff(attempt int) time.Duration {
	base := 500 * time.Millisecond << attempt
	if base > 8*time.Second {
		base = 8 * time.Second
	}
	return base/2 + time.Duration(rand.Int64N(int64(base/2)))
}

func ParseRetryAfter(header http.Header) time.Duration {
	v := header.Get("retry-after")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

func StatusRetryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return true
	}
	return status >= 500
}
