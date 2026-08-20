package openaicompat

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchModels(t *testing.T) {
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("authorization")
		io.WriteString(w, `{"object":"list","data":[
			{"id":"gpt-5.1","object":"model"},
			{"id":"gpt-4o","object":"model"},
			{"id":"","object":"model"}
		]}`)
	}))
	defer srv.Close()

	c, err := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "gpt-4o,gpt-5.1" {
		t.Errorf("got %v, want them sorted with the blank dropped", got)
	}
	if path != "/models" {
		t.Errorf("path = %q", path)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("authorization = %q", auth)
	}
}

func TestFetchModelsOnAnEmptyAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"object":"list","data":[]}`)
	}))
	defer srv.Close()

	c, _ := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestFetchModelsReportsABadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	c, _ := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if _, err := c.FetchModels(context.Background()); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestFetchModelsReportsAnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"type":"authentication_error","message":"bad key"}}`)
	}))
	defer srv.Close()

	c, _ := New(testConfig, WithAPIKey("sk-test"), WithBaseURL(srv.URL), WithMaxRetries(0))
	_, err := c.FetchModels(context.Background())
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "bad key") {
		t.Errorf("got %v", err)
	}
}

func TestFetchModelsLearnsWhatTheHostCharges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[
			{"id":"vendor/cheap","pricing":{"prompt":"0.0000005","completion":"0.000002","input_cache_read":"0.00000005"}},
			{"id":"vendor/silent"}]}`)
	}))
	defer server.Close()

	c, err := New(Config{Name: "test", BaseURL: server.URL, DefaultModel: "vendor/cheap"}, WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatal(err)
	}

	prices := c.Prices()
	p, ok := prices["vendor/cheap"]
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	if !ok || !near(p.Input, 0.5) || !near(p.Output, 2) || !near(p.CacheRead, 0.05) {
		t.Errorf("cheap = %+v %v, want per-million figures", p, ok)
	}
	if _, ok := prices["vendor/silent"]; ok {
		t.Error("a model with no pricing must not be priced")
	}
}
