package anthropic

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zenodea/zaino/internal/x/httpx"
)

func TestParseAPIErrorReadsTheEnvelope(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	e := parseAPIError(http.StatusTooManyRequests, "req_123", body)

	if e.Type != "rate_limit_error" {
		t.Errorf("type = %q", e.Type)
	}
	if e.Message != "slow down" {
		t.Errorf("message = %q", e.Message)
	}
	if e.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d", e.StatusCode)
	}
	if string(e.Body) != string(body) {
		t.Error("the raw body was not kept")
	}
}

func TestParseAPIErrorFallsBackToTheRawBody(t *testing.T) {
	e := parseAPIError(http.StatusBadGateway, "", []byte("<html>gateway</html>"))
	if e.Type != "unknown_error" {
		t.Errorf("type = %q, want unknown_error", e.Type)
	}
	if e.Message != "<html>gateway</html>" {
		t.Errorf("message = %q", e.Message)
	}
}

func TestParseAPIErrorFallsBackToTheStatusText(t *testing.T) {
	e := parseAPIError(http.StatusServiceUnavailable, "", nil)
	if got := e.Message; got != http.StatusText(http.StatusServiceUnavailable) {
		t.Errorf("message = %q, want the status text", got)
	}
}

// A well-formed JSON body that is not an error envelope must not be mistaken
// for one.
func TestParseAPIErrorIgnoresAnEnvelopeWithoutAType(t *testing.T) {
	e := parseAPIError(http.StatusBadRequest, "", []byte(`{"error":{"message":"no type"}}`))
	if e.Type != "unknown_error" {
		t.Errorf("type = %q, want unknown_error", e.Type)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	cases := []struct {
		err  *APIError
		want string
	}{
		{&APIError{Type: "stream_error", Message: "broken"}, "anthropic: stream_error: broken"},
		{&APIError{StatusCode: 400, Type: "invalid_request_error", Message: "bad"}, "anthropic: 400 invalid_request_error: bad"},
		{&APIError{StatusCode: 400, Type: "invalid_request_error", Message: "bad", RequestID: "req_1"}, "anthropic: 400 invalid_request_error: bad (request-id req_1)"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestAPIErrorRetryabilityFollowsTheStatus(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 408, 409} {
		if !(&APIError{StatusCode: status}).Retryable() {
			t.Errorf("%d should be retryable", status)
		}
	}
	for _, status := range []int{400, 401, 403, 404} {
		if (&APIError{StatusCode: status}).Retryable() {
			t.Errorf("%d should not be retryable", status)
		}
	}
}

func TestAPIErrorSatisfiesTheRetryContracts(t *testing.T) {
	var err error = &APIError{StatusCode: 429, retryAfter: 3 * time.Second}

	var retryable httpx.Retryable
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Error("APIError does not read as retryable to httpx")
	}
	var after httpx.RetryAfter
	if !errors.As(err, &after) {
		t.Fatal("APIError does not carry a retry-after")
	}
	if got := after.RetryAfter(); got != 3*time.Second {
		t.Errorf("retry-after = %v, want 3s", got)
	}
}

func TestNewNeedsCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	_, err := New()
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
	for _, want := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s: %v", want, err)
		}
	}
}

func TestNewTakesCredentialsFromTheEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "sk-env")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.apiKey != "sk-env" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
}

func TestExplicitCredentialsWinOverTheEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-env")

	c, err := New(WithAPIKey("sk-explicit"))
	if err != nil {
		t.Fatal(err)
	}
	if c.apiKey != "sk-explicit" {
		t.Errorf("apiKey = %q, want the explicit one", c.apiKey)
	}
}

func TestAnAuthTokenIsCredentialEnough(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "oauth-token")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.authToken != "oauth-token" {
		t.Errorf("authToken = %q", c.authToken)
	}
}

func TestBaseURLComesFromTheEnvironmentUnlessSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_BASE_URL", "https://proxy.internal")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://proxy.internal" {
		t.Errorf("baseURL = %q", c.baseURL)
	}

	c, err = New(WithBaseURL("https://explicit.example"))
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://explicit.example" {
		t.Errorf("baseURL = %q, want the explicit one", c.baseURL)
	}
}

func TestClientIdentity(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "anthropic" {
		t.Errorf("name = %q", c.Name())
	}
	if c.DefaultModel() == "" {
		t.Error("no default model")
	}
	models := c.Models()
	if len(models) == 0 {
		t.Fatal("no models listed")
	}
	if !strings.Contains(strings.Join(models, ","), c.DefaultModel()) {
		t.Errorf("the default model %q is not among %v", c.DefaultModel(), models)
	}
}

func TestOptionsReachTheClient(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	http := httpx.NewClient()
	c, err := New(WithBetas("beta-a", "beta-b"), WithMaxRetries(7), WithHTTPClient(http))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.betas) != 2 {
		t.Errorf("betas = %v", c.betas)
	}
	if c.http.MaxRetries != 7 {
		t.Errorf("maxRetries = %d, want 7", c.http.MaxRetries)
	}
	if c.http.HTTP != http {
		t.Error("the http client was not installed")
	}
}
