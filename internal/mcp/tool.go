package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/tool"
)

// Names are prefixed with the server they came from, so two servers offering
// "search" stay distinguishable and the transcript says where a call went.
func Tools(client *Client, defs []Definition) []tool.Tool {
	out := make([]tool.Tool, 0, len(defs))
	for _, def := range defs {
		out = append(out, &remote{client: client, def: def})
	}
	return out
}

type remote struct {
	client *Client
	def    Definition
}

func (r *remote) Definition() llm.Tool {
	schema := r.def.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return llm.Tool{
		Name:        r.client.Name + "__" + r.def.Name,
		Description: r.def.Description,
		InputSchema: schema,
	}
}

func (r *remote) Prepare(input json.RawMessage) (tool.Call, error) {
	if len(input) > 0 && !json.Valid(input) {
		return nil, fmt.Errorf("bad arguments")
	}
	return &call{client: r.client, name: r.def.Name, input: input}, nil
}

type call struct {
	client *Client
	name   string
	input  json.RawMessage
}

// Nothing is known about what a server does with a call, so it is asked for
// like anything else that leaves the process.
func (c *call) Request() permission.Request {
	return permission.Request{
		Tool:    c.client.Name + "__" + c.name,
		Action:  permission.Execute,
		Target:  c.client.Name + " " + c.name,
		Preview: preview(c.input),
	}
}

func (c *call) Run(ctx context.Context) (string, error) {
	return c.client.CallTool(ctx, c.name, c.input)
}

func preview(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, input, "", "  "); err != nil {
		return string(input)
	}
	return pretty.String()
}
