package tool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/permission"
)

func fetchFrom(t *testing.T, handler http.HandlerFunc) (*Fetch, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Fetch{client: server.Client()}, server.URL
}

func TestFetchStripsHTML(t *testing.T) {
	f, url := fetchFrom(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(`<html><head><style>p{color:red}</style></head>
			<body><h1>Thought signatures</h1>
			<script>track()</script>
			<p>You <b>must</b> send it back.</p></body></html>`))
	})

	out := mustRun(t, f, fetchArgs{URL: url})
	for _, want := range []string{"Thought signatures", "must", "send it back"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"<p>", "color:red", "track()"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output still carries %q:\n%s", unwanted, out)
		}
	}
}

func TestFetchRawKeepsTheMarkup(t *testing.T) {
	f, url := fetchFrom(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		w.Write([]byte("<p>hello</p>"))
	})

	if out := mustRun(t, f, fetchArgs{URL: url, Raw: true}); !strings.Contains(out, "<p>") {
		t.Errorf("raw fetch stripped the markup:\n%s", out)
	}
}

func TestFetchReportsFailures(t *testing.T) {
	f, url := fetchFrom(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such page", http.StatusNotFound)
	})

	_, err := run(t, f, fetchArgs{URL: url})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want the status", err)
	}
}

func TestFetchRejectsWhatItCannotGet(t *testing.T) {
	tests := []struct{ name, url, want string }{
		{"a file path", "file:///etc/passwd", "only http and https"},
		{"a scheme it cannot speak", "ftp://example.com/x", "only http and https"},
		{"no host", "https://", "no host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := prepare(t, &Fetch{}, fetchArgs{URL: tt.url})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

func TestFetchAsksAboutTheHost(t *testing.T) {
	call, err := prepare(t, &Fetch{}, fetchArgs{URL: "https://ai.google.dev/gemini-api/docs/thinking"})
	if err != nil {
		t.Fatal(err)
	}

	req := call.Request()
	if req.Action != permission.Network {
		t.Errorf("Action = %q, want network", req.Action)
	}
	if req.Target != "ai.google.dev" {
		t.Errorf("Target = %q, want the host — that is what a session grant should cover", req.Target)
	}
}

// Planning is mostly reading, and fetching a page changes nothing locally.
func TestFetchIsAllowedWhilePlanning(t *testing.T) {
	req := permission.Request{Tool: "fetch", Action: permission.Network, Target: "example.com"}

	if got, _ := permission.NewPolicy(permission.Plan).Decide(req); got != permission.Allow {
		t.Errorf("plan mode = %v, want fetch allowed", got)
	}
	if got, _ := permission.NewPolicy(permission.Manual).Decide(req); got != permission.Ask {
		t.Errorf("manual mode = %v, want fetch to ask", got)
	}
	if got, _ := permission.NewPolicy(permission.AcceptEdits).Decide(req); got != permission.Ask {
		t.Errorf("accept-edits = %v, want fetch to ask; edits are local, this is not", got)
	}
}
