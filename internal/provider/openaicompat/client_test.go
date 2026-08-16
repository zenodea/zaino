package openaicompat

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/x/httpx"
)

func TestNewNeedsCredentials(t *testing.T) {
	t.Setenv("TEST_API_KEY", "")

	_, err := New(testConfig)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
	if !strings.Contains(err.Error(), "TEST_API_KEY") {
		t.Errorf("the error does not name the variable: %v", err)
	}
	if strings.Count(err.Error(), "no credentials") != 1 {
		t.Errorf("the error stutters: %v", err)
	}
}

func TestNewReadsTheFirstEnvironmentKeyThatIsSet(t *testing.T) {
	cfg := testConfig
	cfg.APIKeyEnv = []string{"TEST_PRIMARY_KEY", "TEST_FALLBACK_KEY"}

	t.Setenv("TEST_PRIMARY_KEY", "")
	t.Setenv("TEST_FALLBACK_KEY", "from-fallback")
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.apiKey != "from-fallback" {
		t.Errorf("apiKey = %q", c.apiKey)
	}

	t.Setenv("TEST_PRIMARY_KEY", "from-primary")
	c, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.apiKey != "from-primary" {
		t.Errorf("apiKey = %q, want the primary variable to win", c.apiKey)
	}
}

func TestExplicitCredentialsWinOverTheEnvironment(t *testing.T) {
	t.Setenv("TEST_API_KEY", "from-env")

	c, err := New(testConfig, WithAPIKey("explicit"))
	if err != nil {
		t.Fatal(err)
	}
	if c.apiKey != "explicit" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
}

func TestBaseURLComesFromTheEnvironmentUnlessSet(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test")
	t.Setenv("TEST_BASE_URL", "https://proxy.internal/v1")

	c, err := New(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://proxy.internal/v1" {
		t.Errorf("baseURL = %q", c.baseURL)
	}

	c, err = New(testConfig, WithBaseURL("https://explicit.example/v1"))
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://explicit.example/v1" {
		t.Errorf("baseURL = %q, want the explicit one", c.baseURL)
	}
}

func TestClientIdentity(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test")

	c, err := New(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "test" {
		t.Errorf("name = %q", c.Name())
	}
	if c.DefaultModel() != "test-1" {
		t.Errorf("default model = %q", c.DefaultModel())
	}

	models := c.Models()
	if len(models) != 2 {
		t.Fatalf("got %v", models)
	}
	models[0] = "mutated"
	if c.Models()[0] == "mutated" {
		t.Error("Models() handed out the config's own slice")
	}
}

func TestWithDefaultModel(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test")

	c, err := New(testConfig, WithDefaultModel("test-2"))
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultModel() != "test-2" {
		t.Errorf("default model = %q", c.DefaultModel())
	}
}

func TestOptionsReachTheClient(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test")

	client := httpx.NewClient()
	c, err := New(testConfig, WithMaxRetries(7), WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	if c.http.MaxRetries != 7 {
		t.Errorf("maxRetries = %d, want 7", c.http.MaxRetries)
	}
	if c.http.HTTP != client {
		t.Error("the http client was not installed")
	}
}

func TestParseAPIError(t *testing.T) {
	e := parseAPIError("test", 429, "req_1", []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	if e.Type != "rate_limit_error" || e.Message != "slow down" {
		t.Errorf("got %+v", e)
	}
	if !strings.Contains(e.Error(), "req_1") {
		t.Errorf("the request id is missing: %v", e)
	}
	if !e.Retryable() {
		t.Error("a 429 should be retryable")
	}
}

func TestParseAPIErrorPrefersTheCodeOverTheType(t *testing.T) {
	e := parseAPIError("test", 404, "", []byte(`{"error":{"type":"invalid_request_error","code":"model_not_found","message":"nope"}}`))
	if !strings.Contains(e.Error(), "model_not_found") {
		t.Errorf("got %v", e)
	}
}

// Some hosts send a numeric code where OpenAI sends a string.
func TestParseAPIErrorToleratesANumericCode(t *testing.T) {
	e := parseAPIError("test", 400, "", []byte(`{"error":{"type":"invalid_request_error","code":400,"message":"bad"}}`))
	if e.Code != "" {
		t.Errorf("code = %q, want it left alone", e.Code)
	}
	if e.Message != "bad" {
		t.Errorf("message = %q", e.Message)
	}
}

func TestParseAPIErrorFallsBackToTheRawBody(t *testing.T) {
	e := parseAPIError("test", 502, "", []byte("<html>gateway</html>"))
	if e.Type != "unknown_error" || e.Message != "<html>gateway</html>" {
		t.Errorf("got %+v", e)
	}

	e = parseAPIError("test", 503, "", nil)
	if e.Message != "Service Unavailable" {
		t.Errorf("message = %q", e.Message)
	}
}

func TestAPIErrorWithoutAStatus(t *testing.T) {
	e := &APIError{Provider: "test", Type: "stream_error", Message: "broken"}
	if got, want := e.Error(), "test: stream_error: broken"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAPIErrorSatisfiesTheRetryContracts(t *testing.T) {
	var err error = &APIError{StatusCode: 500}
	var retryable httpx.Retryable
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Error("APIError does not read as retryable to httpx")
	}
	var after httpx.RetryAfter
	if !errors.As(err, &after) {
		t.Error("APIError does not carry a retry-after")
	}
}
