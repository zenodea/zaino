package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	protocolVersion = "2024-11-05"
	callTimeout     = 2 * time.Minute
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// Client speaks JSON-RPC over one duplex stream. Everything a server sends
// that is not a reply to an outstanding call is dropped: zaino asks questions,
// it does not serve them.
type Client struct {
	Name string

	conn io.ReadWriteCloser
	out  *json.Encoder

	seq     atomic.Int64
	writing sync.Mutex

	pending sync.Map

	closeOnce sync.Once
	done      chan struct{}
	readErr   error
}

func New(name string, conn io.ReadWriteCloser) *Client {
	c := &Client{
		Name: name,
		conn: conn,
		out:  json.NewEncoder(conn),
		done: make(chan struct{}),
	}
	go c.read()
	return c
}

func (c *Client) read() {
	defer close(c.done)

	lines := bufio.NewScanner(c.conn)
	lines.Buffer(make([]byte, 0, 64<<10), 8<<20)

	for lines.Scan() {
		var resp response
		if err := json.Unmarshal(lines.Bytes(), &resp); err != nil || resp.ID == 0 {
			continue
		}
		if waiting, ok := c.pending.LoadAndDelete(resp.ID); ok {
			waiting.(chan response) <- resp
		}
	}
	c.readErr = lines.Err()
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = encoded
	}

	id := c.seq.Add(1)
	reply := make(chan response, 1)
	c.pending.Store(id, reply)
	defer c.pending.Delete(id)

	c.writing.Lock()
	err := c.out.Encode(request{JSONRPC: "2.0", ID: id, Method: method, Params: raw})
	c.writing.Unlock()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.Name, err)
	}

	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	select {
	case resp := <-reply:
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %w", c.Name, resp.Error)
		}
		return resp.Result, nil
	case <-c.done:
		if c.readErr != nil {
			return nil, fmt.Errorf("%s stopped: %w", c.Name, c.readErr)
		}
		return nil, fmt.Errorf("%s stopped", c.Name)
	case <-ctx.Done():
		return nil, fmt.Errorf("%s did not answer %s: %w", c.Name, method, ctx.Err())
	}
}

func (c *Client) notify(method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = encoded
	}

	c.writing.Lock()
	defer c.writing.Unlock()
	return c.out.Encode(request{JSONRPC: "2.0", Method: method, Params: raw})
}

type Info struct {
	ProtocolVersion string                         `json:"protocolVersion"`
	ServerInfo      struct{ Name, Version string } `json:"serverInfo"`
	Capabilities    map[string]any                 `json:"capabilities"`
}

func (c *Client) Initialize(ctx context.Context) (Info, error) {
	raw, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "zaino", "version": "0"},
	})
	if err != nil {
		return Info{}, err
	}

	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return Info{}, fmt.Errorf("%s: %w", c.Name, err)
	}
	return info, c.notify("notifications/initialized", map[string]any{})
}

type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (c *Client) ListTools(ctx context.Context) ([]Definition, error) {
	var all []Definition
	cursor := ""

	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}

		var page struct {
			Tools      []Definition `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("%s: %w", c.Name, err)
		}

		all = append(all, page.Tools...)
		if page.NextCursor == "" || page.NextCursor == cursor {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

type Result struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// A server reports a failed tool with isError rather than a JSON-RPC error,
// so the text is what the model should see either way.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}

	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}

	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("%s: %w", c.Name, err)
	}

	var text []string
	for _, block := range result.Content {
		if block.Text != "" {
			text = append(text, block.Text)
		}
	}
	joined := joinLines(text)

	if result.IsError {
		return "", fmt.Errorf("%s", orElse(joined, "the server reported an error"))
	}
	return orElse(joined, "(no output)"), nil
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() { err = c.conn.Close() })
	return err
}

func joinLines(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\n"
		}
		out += p
	}
	return out
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
