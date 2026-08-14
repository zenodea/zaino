package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

// A server that answers over a pipe, so the wire format is exercised without
// spawning anything.
type fakeServer struct {
	mu     sync.Mutex
	tools  []Definition
	pages  bool
	result Result
	rpcErr *rpcError
	calls  []string
}

type duplex struct {
	io.Reader
	io.WriteCloser
}

func (d duplex) Close() error { return d.WriteCloser.Close() }

func serve(t *testing.T, s *fakeServer) *Client {
	t.Helper()

	toClient, serverWrites := io.Pipe()
	toServer, clientWrites := io.Pipe()

	go func() {
		defer serverWrites.Close()
		lines := bufio.NewScanner(toServer)
		out := json.NewEncoder(serverWrites)

		for lines.Scan() {
			var req request
			if json.Unmarshal(lines.Bytes(), &req) != nil || req.ID == 0 {
				continue
			}

			s.mu.Lock()
			s.calls = append(s.calls, req.Method)
			s.mu.Unlock()

			resp := response{JSONRPC: "2.0", ID: req.ID}
			switch req.Method {
			case "initialize":
				resp.Result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"fake","version":"1"}}`)
			case "tools/list":
				resp.Result = s.listing(req.Params)
			case "tools/call":
				if s.rpcErr != nil {
					resp.Error = s.rpcErr
					break
				}
				body, _ := json.Marshal(s.result)
				resp.Result = body
			}
			out.Encode(resp)
		}
	}()

	client := New("fake", duplex{Reader: toClient, WriteCloser: clientWrites})
	t.Cleanup(func() { client.Close() })
	return client
}

func (s *fakeServer) listing(params json.RawMessage) json.RawMessage {
	var p struct {
		Cursor string `json:"cursor"`
	}
	json.Unmarshal(params, &p)

	if !s.pages {
		body, _ := json.Marshal(map[string]any{"tools": s.tools})
		return body
	}
	if p.Cursor == "" {
		body, _ := json.Marshal(map[string]any{"tools": s.tools[:1], "nextCursor": "more"})
		return body
	}
	body, _ := json.Marshal(map[string]any{"tools": s.tools[1:]})
	return body
}

func textResult(text string) Result {
	var r Result
	r.Content = append(r.Content, struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "text", Text: text})
	return r
}

func TestHandshakeAndListing(t *testing.T) {
	c := serve(t, &fakeServer{tools: []Definition{
		{Name: "search", Description: "search the docs", InputSchema: map[string]any{"type": "object"}},
	}})

	info, err := c.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if info.ServerInfo.Name != "fake" {
		t.Errorf("server name = %q", info.ServerInfo.Name)
	}

	defs, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "search" {
		t.Errorf("tools = %+v", defs)
	}
}

func TestListingFollowsPages(t *testing.T) {
	c := serve(t, &fakeServer{pages: true, tools: []Definition{{Name: "one"}, {Name: "two"}}})

	defs, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(defs) != 2 {
		t.Errorf("got %d tools, want both pages", len(defs))
	}
}

func TestCallReturnsText(t *testing.T) {
	c := serve(t, &fakeServer{result: textResult("found it")})

	out, err := c.CallTool(context.Background(), "search", json.RawMessage(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out != "found it" {
		t.Errorf("out = %q", out)
	}
}

// A server reports a failed tool with isError, not a JSON-RPC error, and the
// model needs to see the text either way.
func TestToolErrorBecomesAnError(t *testing.T) {
	failed := textResult("no such document")
	failed.IsError = true
	c := serve(t, &fakeServer{result: failed})

	_, err := c.CallTool(context.Background(), "search", nil)
	if err == nil || !strings.Contains(err.Error(), "no such document") {
		t.Errorf("err = %v, want the server's own words", err)
	}
}

func TestProtocolErrorIsNamed(t *testing.T) {
	c := serve(t, &fakeServer{rpcErr: &rpcError{Code: -32601, Message: "method not found"}})

	_, err := c.CallTool(context.Background(), "search", nil)
	if err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "fake") {
		t.Errorf("err = %v, want the server named", err)
	}
}

func TestAServerThatDiesDoesNotHang(t *testing.T) {
	c := serve(t, &fakeServer{})
	c.Close()

	if _, err := c.CallTool(context.Background(), "search", nil); err == nil {
		t.Error("calling a closed server returned no error")
	}
}

func TestRemoteToolsCarryTheServerName(t *testing.T) {
	c := serve(t, &fakeServer{})
	tools := Tools(c, []Definition{{Name: "search", Description: "look"}})

	if len(tools) != 1 {
		t.Fatalf("got %d tools", len(tools))
	}
	def := tools[0].Definition()
	if def.Name != "fake__search" {
		t.Errorf("name = %q, want it prefixed with the server", def.Name)
	}
	if def.InputSchema["type"] != "object" {
		t.Errorf("schema = %v, want an object even when the server sent none", def.InputSchema)
	}
}

// Nothing is known about what a server does, so it is asked for like anything
// else that leaves the process.
func TestRemoteToolsAsk(t *testing.T) {
	c := serve(t, &fakeServer{})
	tools := Tools(c, []Definition{{Name: "deploy"}})

	call, err := tools[0].Prepare(json.RawMessage(`{"target":"prod"}`))
	if err != nil {
		t.Fatal(err)
	}
	req := call.Request()
	if req.Action != permission.Execute {
		t.Errorf("Action = %q, want execute", req.Action)
	}
	if !strings.Contains(req.Preview, "prod") {
		t.Errorf("Preview = %q, want the arguments shown", req.Preview)
	}
}

func TestLoadMissingConfigIsNotAnError(t *testing.T) {
	cfg, err := Load(t.TempDir() + "/absent.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("got %d servers from nothing", len(cfg.Servers))
	}
}

func TestConfigNamesAreStable(t *testing.T) {
	cfg := Config{Servers: map[string]Server{"zeta": {}, "alpha": {}, "mid": {}}}
	got := strings.Join(cfg.Names(), ",")
	if got != "alpha,mid,zeta" {
		t.Errorf("Names() = %q, want them sorted so the tool list holds still", got)
	}
}

// No servers is the ordinary case: there is usually no mcp.json at all, and
// the session that stands for "none" has to be usable without checking it.
func TestAnEmptySessionIsUsable(t *testing.T) {
	var none *Session

	if got := none.All(); got != nil {
		t.Errorf("All() = %v, want nothing", got)
	}
	none.Close()

	tools := append([]string{"read"}, namesOf(none.All())...)
	if len(tools) != 1 {
		t.Errorf("appending an empty session's tools changed the list: %v", tools)
	}
}

func TestConnectWithNoServersYieldsAnEmptySession(t *testing.T) {
	session, failures := Connect(context.Background(), Config{})
	if len(failures) != 0 {
		t.Errorf("failures = %v, want none", failures)
	}
	if got := session.All(); len(got) != 0 {
		t.Errorf("All() = %v, want no tools", got)
	}
	session.Close()
}

func namesOf(tools []tool.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Definition().Name)
	}
	return out
}
