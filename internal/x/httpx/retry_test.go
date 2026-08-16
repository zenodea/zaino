package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type testError struct {
	status int
	after  time.Duration
}

func (e *testError) Error() string             { return fmt.Sprintf("status %d", e.status) }
func (e *testError) Retryable() bool           { return StatusRetryable(e.status) }
func (e *testError) RetryAfter() time.Duration { return e.after }

// RetryAfter is honoured over the backoff, which keeps these tests quick.
func newTestClient(h http.Handler, maxRetries int) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	return &Client{
		HTTP:       srv.Client(),
		MaxRetries: maxRetries,
		ParseError: func(status int, header http.Header, body []byte) error {
			return &testError{status: status, after: time.Millisecond}
		},
	}, srv
}

func TestPostReturnsASuccessfulResponse(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("content-type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"n":1}` {
			t.Errorf("body = %s", body)
		}
		fmt.Fprint(w, "ok")
	}), 2)
	defer srv.Close()

	resp, err := c.Post(context.Background(), srv.URL, nil, []byte(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got, _ := io.ReadAll(resp.Body); string(got) != "ok" {
		t.Errorf("got %s, want ok", got)
	}
}

func TestPostSetsCallerHeaders(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Errorf("x-api-key = %q", got)
		}
	}), 0)
	defer srv.Close()

	resp, err := c.Post(context.Background(), srv.URL, func(req *http.Request) {
		req.Header.Set("x-api-key", "secret")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestPostRetriesUntilItSucceeds(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, "finally")
	}), 3)
	defer srv.Close()

	resp, err := c.Post(context.Background(), srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := calls.Load(); got != 3 {
		t.Errorf("made %d calls, want 3", got)
	}
}

func TestPostGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}), 2)
	defer srv.Close()

	_, err := c.Post(context.Background(), srv.URL, nil, nil)
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("made %d calls, want 3 (the first plus two retries)", got)
	}
}

// A 400 is the caller's fault; sending it again would fail the same way.
func TestPostDoesNotRetryAClientError(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}), 5)
	defer srv.Close()

	if _, err := c.Post(context.Background(), srv.URL, nil, nil); err == nil {
		t.Fatal("got nil, want an error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("made %d calls, want 1", got)
	}
}

func TestPostSurfacesTheParsedError(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}), 0)
	defer srv.Close()

	_, err := c.Post(context.Background(), srv.URL, nil, nil)
	var api *testError
	if !errors.As(err, &api) {
		t.Fatalf("got %T (%v), want *testError", err, err)
	}
	if api.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", api.status)
	}
}

func TestPostStopsWhenTheContextIsCancelled(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}), 20)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Post(ctx, srv.URL, nil, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("got nil, want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Post kept retrying a cancelled request")
	}
}

func TestPostReportsAnUnreachableServer(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), 0)
	url := srv.URL
	srv.Close()

	if _, err := c.Post(context.Background(), url, nil, nil); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestPostRejectsABadURL(t *testing.T) {
	c := &Client{HTTP: NewClient()}
	if _, err := c.Post(context.Background(), "://nope", nil, nil); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestStatusRetryable(t *testing.T) {
	cases := map[int]bool{
		http.StatusOK:                  false,
		http.StatusBadRequest:          false,
		http.StatusUnauthorized:        false,
		http.StatusForbidden:           false,
		http.StatusNotFound:            false,
		http.StatusRequestTimeout:      true,
		http.StatusConflict:            true,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
		http.StatusGatewayTimeout:      true,
	}
	for status, want := range cases {
		if got := StatusRetryable(status); got != want {
			t.Errorf("StatusRetryable(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"3", 3 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0},
		{"nonsense", 0},
	}
	for _, c := range cases {
		h := http.Header{}
		if c.value != "" {
			h.Set("retry-after", c.value)
		}
		if got := ParseRetryAfter(h); got != c.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	for attempt := range 10 {
		base := min(500*time.Millisecond<<attempt, 8*time.Second)
		for range 50 {
			got := Backoff(attempt)
			if got < base/2 || got >= base {
				t.Fatalf("Backoff(%d) = %v, want [%v, %v)", attempt, got, base/2, base)
			}
		}
	}
}

func TestBackoffJitters(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 50 {
		seen[Backoff(3)] = true
	}
	if len(seen) < 2 {
		t.Error("Backoff returned one fixed value, expected jitter")
	}
}

func TestShouldRetry(t *testing.T) {
	if shouldRetry(context.Canceled) {
		t.Error("a cancelled context should not be retried")
	}
	if shouldRetry(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)) {
		t.Error("a deadline should not be retried")
	}
	if !shouldRetry(errors.New("connection reset")) {
		t.Error("an unclassified transport error should be retried")
	}
	if shouldRetry(&testError{status: http.StatusBadRequest}) {
		t.Error("a 400 should not be retried")
	}
	if !shouldRetry(&testError{status: http.StatusTooManyRequests}) {
		t.Error("a 429 should be retried")
	}
}

func TestNewTransportBoundsItsDials(t *testing.T) {
	tr := NewTransport()
	if tr.TLSHandshakeTimeout == 0 || tr.ResponseHeaderTimeout == 0 {
		t.Error("transport has no timeouts")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("http2 is off")
	}
	if got := NewClient().Timeout; got != 0 {
		t.Errorf("client timeout = %v, want 0 — a streaming turn runs for minutes", got)
	}
}

func TestGetSendsNoBody(t *testing.T) {
	var method string
	var hadContentType bool
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_, hadContentType = r.Header["Content-Type"]
		fmt.Fprint(w, "ok")
	}), 0)
	defer srv.Close()

	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if method != http.MethodGet {
		t.Errorf("method = %q", method)
	}
	if hadContentType {
		t.Error("a GET carried a content-type")
	}
}

func TestGetRetriesLikePost(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "ok")
	}), 2)
	defer srv.Close()

	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := calls.Load(); got != 2 {
		t.Errorf("made %d calls, want 2", got)
	}
}
