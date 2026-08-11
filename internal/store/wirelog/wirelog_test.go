package wirelog

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	var out []map[string]any
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scan.Bytes(), &rec); err != nil {
			t.Fatalf("bad log line %q: %v", scan.Text(), err)
		}
		out = append(out, rec)
	}
	return out
}

func TestRecordsRequestAndStreamedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		io.WriteString(w, "data: two\n\n")
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "wire.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	client := &http.Client{Transport: log.Transport(nil)}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"x"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("x-api-key", "sk-ant-secret")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "data: two") {
		t.Fatalf("the tee ate the body: %q", body)
	}
	log.Close()

	records := read(t, path)
	kinds := map[string]int{}
	for _, rec := range records {
		kinds[rec["kind"].(string)]++
	}
	for _, want := range []string{"request", "response", "chunk", "body_end"} {
		if kinds[want] == 0 {
			t.Errorf("no %q record; got %v", want, kinds)
		}
	}

	request := records[0]
	headers := request["headers"].(map[string]any)
	if headers["x-api-key"] != "[redacted]" {
		t.Errorf("x-api-key = %v, want it redacted", headers["x-api-key"])
	}
	if headers["anthropic-version"] != "2023-06-01" {
		t.Errorf("ordinary headers should survive: %v", headers)
	}
	if sent, ok := request["body"].(map[string]any); !ok || sent["model"] != "x" {
		t.Errorf("request body = %v, want the JSON we sent", request["body"])
	}

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sk-ant-secret") {
		t.Error("the api key reached the log file")
	}
}

func TestRedactsKeyInQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wire.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	client := &http.Client{Transport: log.Transport(errorTripper{})}
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/v1?key=secret-key&alt=sse", nil)
	client.Do(req)
	log.Close()

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "secret-key") {
		t.Error("a key in the query string reached the log file")
	}
	if !strings.Contains(string(raw), "alt=sse") {
		t.Error("the rest of the query should survive")
	}
	if !strings.Contains(string(raw), `"kind":"error"`) {
		t.Error("a failed request should be recorded")
	}
}

type errorTripper struct{}

func (errorTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestNilLogIsInert(t *testing.T) {
	var log *Log
	if got := log.Transport(nil); got != nil {
		t.Errorf("Transport(nil) on a nil log = %v, want nil", got)
	}
	existing := errorTripper{}
	if got := log.Transport(existing); got != http.RoundTripper(existing) {
		t.Errorf("Transport on a nil log = %v, want the transport back unwrapped", got)
	}
	log.Note("no panic")
	if err := log.Close(); err != nil {
		t.Errorf("Close on a nil log = %v", err)
	}
}
