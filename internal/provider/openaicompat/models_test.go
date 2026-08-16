package openaicompat

import (
	"context"
	"io"
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
