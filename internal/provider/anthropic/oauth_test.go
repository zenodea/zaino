package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func headerCapture(t *testing.T) (*httptest.Server, *http.Header) {
	t.Helper()
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestAnAPIKeyGoesOutAsXAPIKey(t *testing.T) {
	srv, seen := headerCapture(t)

	c, err := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	if got := seen.Get("x-api-key"); got != "sk-test" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := seen.Get("authorization"); got != "" {
		t.Errorf("authorization = %q — sending both has the API reject it", got)
	}
	if slices.Contains(seen.Values("anthropic-beta"), oauthBeta) {
		t.Error("the oauth beta went out with an api key")
	}
}

// OAuth tokens go on Authorization: Bearer, and /v1/messages rejects them
// without the oauth beta header.
func TestATokenGoesOutAsABearerWithTheOAuthBeta(t *testing.T) {
	srv, seen := headerCapture(t)

	c, err := New(WithAuthToken("oat-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	if got := seen.Get("authorization"); got != "Bearer oat-test" {
		t.Errorf("authorization = %q", got)
	}
	if got := seen.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key = %q — sending both has the API reject it", got)
	}
	if !slices.Contains(seen.Values("anthropic-beta"), oauthBeta) {
		t.Errorf("anthropic-beta = %v, want it to include %s", seen.Values("anthropic-beta"), oauthBeta)
	}
}

func TestATokenSourceIsCredentialEnough(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	c, err := New(WithTokenSource(func(context.Context) (string, error) {
		return "oat-from-source", nil
	}))
	if err != nil {
		t.Fatalf("a token source should count as credentials: %v", err)
	}
	if c.apiKey != "" {
		t.Errorf("apiKey = %q, want none", c.apiKey)
	}
}

// A short-lived token captured once would go stale partway through a session.
func TestTheTokenSourceIsAskedOnEveryRequest(t *testing.T) {
	srv, seen := headerCapture(t)

	var calls atomic.Int32
	c, err := New(
		WithBaseURL(srv.URL),
		WithTokenSource(func(context.Context) (string, error) {
			n := calls.Add(1)
			return "oat-" + string(rune('0'+n)), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		st, err := c.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatal(err)
		}
		st.Close()
	}

	if got := calls.Load(); got != 3 {
		t.Errorf("the source was asked %d times, want 3", got)
	}
	if got := seen.Get("authorization"); got != "Bearer oat-3" {
		t.Errorf("authorization = %q, want the freshest token", got)
	}
}

func TestAFailingTokenSourceStopsTheRequest(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	defer srv.Close()

	boom := errors.New("not logged in")
	c, err := New(WithBaseURL(srv.URL), WithTokenSource(func(context.Context) (string, error) {
		return "", boom
	}))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Stream(context.Background(), llm.Request{}); !errors.Is(err, boom) {
		t.Fatalf("got %v, want the source's error", err)
	}
	if reached.Load() {
		t.Error("the request went out without a credential")
	}
}

func TestAnEmptyTokenIsNoCredential(t *testing.T) {
	c, err := New(WithBaseURL("https://unused.example"), WithTokenSource(func(context.Context) (string, error) {
		return "", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Stream(context.Background(), llm.Request{}); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestATokenSourceOutranksTheEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	srv, seen := headerCapture(t)

	c, err := New(WithBaseURL(srv.URL), WithTokenSource(func(context.Context) (string, error) {
		return "oat-explicit", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	if got := seen.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key = %q — the env key shadowed the token source", got)
	}
	if got := seen.Get("authorization"); got != "Bearer oat-explicit" {
		t.Errorf("authorization = %q", got)
	}
}

func TestConfiguredBetasSurviveTheOAuthBeta(t *testing.T) {
	srv, seen := headerCapture(t)

	c, err := New(WithAuthToken("oat"), WithBaseURL(srv.URL), WithBetas("my-beta-2026-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	betas := seen.Values("anthropic-beta")
	for _, want := range []string{"my-beta-2026-01-01", oauthBeta} {
		if !slices.Contains(betas, want) {
			t.Errorf("anthropic-beta = %v, want it to include %s", betas, want)
		}
	}
}

// The betas slice must not grow by one on every request.
func TestTheOAuthBetaIsNotAppendedTwice(t *testing.T) {
	srv, seen := headerCapture(t)

	c, err := New(WithAuthToken("oat"), WithBaseURL(srv.URL), WithBetas("my-beta"))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		st, err := c.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatal(err)
		}
		st.Close()
	}

	if got := len(seen.Values("anthropic-beta")); got != 2 {
		t.Errorf("sent %d beta headers, want 2: %v", got, seen.Values("anthropic-beta"))
	}
	if got := len(c.betas); got != 1 {
		t.Errorf("the client's betas grew to %v", c.betas)
	}
}
