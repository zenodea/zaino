package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchModelsFollowsPages(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		if r.URL.Query().Get("after_id") == "" {
			io.WriteString(w, `{"data":[{"id":"claude-opus-5"},{"id":"claude-sonnet-5"}],"has_more":true,"last_id":"claude-sonnet-5"}`)
			return
		}
		io.WriteString(w, `{"data":[{"id":"claude-haiku-4-5"}],"has_more":false,"last_id":"claude-haiku-4-5"}`)
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "claude-opus-5,claude-sonnet-5,claude-haiku-4-5"; strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
	if len(paths) != 2 {
		t.Fatalf("made %d requests, want 2: %v", len(paths), paths)
	}
	if !strings.Contains(paths[1], "after_id=claude-sonnet-5") {
		t.Errorf("the second page did not carry the cursor: %q", paths[1])
	}
}

func TestFetchModelsStopsWithoutACursor(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"data":[{"id":"claude-opus-5"}],"has_more":true,"last_id":""}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d requests — has_more without a cursor must not loop", calls)
	}
}

func TestFetchModelsAuthenticates(t *testing.T) {
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-api-key")
		io.WriteString(w, `{"data":[],"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL))
	if _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if key != "sk-test" {
		t.Errorf("x-api-key = %q", key)
	}
}

func TestFetchModelsUsesTheTokenSource(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("authorization")
		io.WriteString(w, `{"data":[],"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := New(WithBaseURL(srv.URL), WithTokenSource(func(context.Context) (string, error) {
		return "oat-live", nil
	}))
	if _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer oat-live" {
		t.Errorf("authorization = %q", auth)
	}
}

func TestFetchModelsReportsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"bad key"}}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("sk-test"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if _, err := c.FetchModels(context.Background()); err == nil {
		t.Fatal("got nil, want an error")
	}
}

// The static list is the offline fallback, so it must not name anything the
// client would then reject.
func TestTheStaticListIncludesTheDefault(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}

	models := c.Models()
	if len(models) < 4 {
		t.Errorf("only %d models listed: %v", len(models), models)
	}
	seen := map[string]bool{}
	for _, id := range models {
		if seen[id] {
			t.Errorf("%q is listed twice", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "claude-") {
			t.Errorf("%q is not a claude model id", id)
		}
	}
	if !seen[c.DefaultModel()] {
		t.Errorf("the default %q is not in the list", c.DefaultModel())
	}
}
