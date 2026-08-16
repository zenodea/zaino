package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchModelsStripsThePrefixAndFiltersKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[
			{"name":"models/gemini-2.5-pro","supportedGenerationMethods":["generateContent","countTokens"]},
			{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]},
			{"name":"models/text-embedding-004","supportedGenerationMethods":["embedContent"]}
		]}`)
	}))
	defer srv.Close()

	c, err := New(WithAPIKey("goog-test"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "gemini-2.5-flash,gemini-2.5-pro"; strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s — an embedding model cannot hold a conversation", got, want)
	}
}

func TestFetchModelsFollowsPages(t *testing.T) {
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.URL.Query().Get("pageToken"))
		if r.URL.Query().Get("pageToken") == "" {
			io.WriteString(w, `{"models":[{"name":"models/a","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"more"}`)
			return
		}
		io.WriteString(w, `{"models":[{"name":"models/b","supportedGenerationMethods":["generateContent"]}]}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("goog-test"), WithBaseURL(srv.URL))
	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("got %v", got)
	}
	if len(tokens) != 2 || tokens[1] != "more" {
		t.Errorf("pagination = %v", tokens)
	}
}

func TestFetchModelsKeepsOnesThatDeclareNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"name":"models/gemini-new"}]}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("goog-test"), WithBaseURL(srv.URL))
	got, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "gemini-new" {
		t.Errorf("got %v — an unannotated model should not be dropped", got)
	}
}

func TestFetchModelsAuthenticates(t *testing.T) {
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-goog-api-key")
		io.WriteString(w, `{"models":[]}`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("goog-test"), WithBaseURL(srv.URL))
	if _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if key != "goog-test" {
		t.Errorf("x-goog-api-key = %q", key)
	}
}

func TestFetchModelsReportsABadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	c, _ := New(WithAPIKey("goog-test"), WithBaseURL(srv.URL))
	if _, err := c.FetchModels(context.Background()); err == nil {
		t.Fatal("got nil, want an error")
	}
}
