package wirelog

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/zenodea/zaino/internal/llm"
)

const MaxBody = 1 << 20

var secretHeaders = map[string]bool{
	"authorization":       true,
	"x-api-key":           true,
	"x-goog-api-key":      true,
	"cookie":              true,
	"set-cookie":          true,
	"proxy-authorization": true,
}

type Log struct {
	mu  sync.Mutex
	f   *os.File
	seq int
}

func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Log{f: f}, nil
}

func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func (l *Log) Turn(model string, stop llm.StopReason, u llm.Usage) {
	l.write(map[string]any{
		"kind":  "turn",
		"model": model,
		"stop":  string(stop),
		"usage": u,
	})
}

func (l *Log) Note(text string) {
	l.write(map[string]any{"kind": "note", "text": text})
}

func (l *Log) Transport(next http.RoundTripper) http.RoundTripper {
	if l == nil {
		return next
	}
	if next == nil {
		next = http.DefaultTransport
	}
	return &transport{log: l, next: next}
}

type transport struct {
	log  *Log
	next http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := t.log.nextID()

	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}

	t.log.write(map[string]any{
		"kind":    "request",
		"id":      id,
		"method":  req.Method,
		"url":     redactURL(req.URL),
		"headers": redactHeaders(req.Header),
		"body":    json.RawMessage(orNull(body)),
	})

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		t.log.write(map[string]any{"kind": "error", "id": id, "error": err.Error()})
		return nil, err
	}

	t.log.write(map[string]any{
		"kind":    "response",
		"id":      id,
		"status":  resp.StatusCode,
		"headers": redactHeaders(resp.Header),
	})

	// Teed as it is read: a turn that dies half way still leaves a record.
	resp.Body = &tee{log: t.log, id: id, src: resp.Body}
	return resp, nil
}

type tee struct {
	log     *Log
	id      int
	src     io.ReadCloser
	written int
	done    bool
}

func (t *tee) Read(p []byte) (int, error) {
	n, err := t.src.Read(p)
	if n > 0 {
		chunk := p[:n]
		if room := MaxBody - t.written; room < len(chunk) {
			chunk = chunk[:max(room, 0)]
		}
		if len(chunk) > 0 {
			t.log.write(map[string]any{
				"kind":  "chunk",
				"id":    t.id,
				"bytes": string(chunk),
			})
		}
		t.written += n
	}
	if err != nil && !t.done {
		t.done = true
		t.log.write(map[string]any{
			"kind":     "body_end",
			"id":       t.id,
			"bytes":    t.written,
			"error":    errText(err),
			"complete": err == io.EOF,
		})
	}
	return n, err
}

func (t *tee) Close() error {
	if !t.done {
		t.done = true
		t.log.write(map[string]any{
			"kind":     "body_end",
			"id":       t.id,
			"bytes":    t.written,
			"complete": false,
		})
	}
	return t.src.Close()
}

func (l *Log) nextID() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	return l.seq
}

func (l *Log) write(record map[string]any) {
	if l == nil {
		return
	}
	record["at"] = time.Now().UTC().Format(time.RFC3339Nano)

	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}
	l.f.Write(append(line, '\n'))
}

func redactHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		if secretHeaders[lower(name)] {
			out[lower(name)] = "[redacted]"
			continue
		}
		joined := ""
		for i, v := range values {
			if i > 0 {
				joined += ", "
			}
			joined += v
		}
		out[lower(name)] = joined
	}
	return out
}

func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	if q := clone.Query(); len(q) > 0 {
		changed := false
		for name := range q {
			switch lower(name) {
			case "key", "api_key", "apikey", "access_token", "token":
				q.Set(name, "[redacted]")
				changed = true
			}
		}
		if changed {
			clone.RawQuery = q.Encode()
		}
	}
	return clone.String()
}

func orNull(body []byte) []byte {
	if !json.Valid(body) {
		quoted, err := json.Marshal(string(body))
		if err != nil {
			return []byte("null")
		}
		return quoted
	}
	return body
}

func errText(err error) string {
	if err == nil || err == io.EOF {
		return ""
	}
	return err.Error()
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
