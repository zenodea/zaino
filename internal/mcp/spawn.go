package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"

	"github.com/zenodea/zaino/internal/tool"
	"github.com/zenodea/zaino/internal/x/paths"
)

type Server struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type Config struct {
	Servers map[string]Server `json:"servers"`
}

func ConfigPath() (string, error) { return paths.Config("mcp.json") }

// A missing config is the normal case, not a failure.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Later files win a name outright: a project pointing "github" somewhere else
// means that server, not both of them.
func LoadAll(paths ...string) (Config, error) {
	merged := Config{}
	for _, path := range paths {
		cfg, err := Load(path)
		if err != nil {
			return Config{}, err
		}
		for name, server := range cfg.Servers {
			if merged.Servers == nil {
				merged.Servers = map[string]Server{}
			}
			merged.Servers[name] = server
		}
	}
	return merged, nil
}

func (c Config) Names() []string {
	out := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type pipe struct {
	io.ReadCloser
	io.WriteCloser
	cmd *exec.Cmd
}

func (p *pipe) Close() error {
	p.WriteCloser.Close()
	p.ReadCloser.Close()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	return p.cmd.Wait()
}

// Spawn starts a server and completes the handshake. Its stderr is left
// attached to ours: a server that cannot start should say so where it is read.
func Spawn(ctx context.Context, name string, server Server) (*Client, []Definition, error) {
	cmd := exec.Command(server.Command, server.Args...)
	cmd.Env = os.Environ()
	for key, value := range server.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}

	client := New(name, &pipe{ReadCloser: stdout, WriteCloser: stdin, cmd: cmd})
	if _, err := client.Initialize(ctx); err != nil {
		client.Close()
		return nil, nil, err
	}

	defs, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	return client, defs, nil
}

type Session struct {
	Clients []*Client
	Tools   []tool.Tool
}

// One server failing is not worth losing the others over, so its error is
// returned alongside whatever did come up.
func Connect(ctx context.Context, cfg Config) (*Session, []error) {
	session := &Session{}
	var failures []error

	for _, name := range cfg.Names() {
		client, defs, err := Spawn(ctx, name, cfg.Servers[name])
		if err != nil {
			failures = append(failures, err)
			continue
		}
		session.Clients = append(session.Clients, client)
		session.Tools = append(session.Tools, Tools(client, defs)...)
	}
	return session, failures
}

// Nil-safe like Close: no servers is the ordinary case, not a special one.
func (s *Session) All() []tool.Tool {
	if s == nil {
		return nil
	}
	return s.Tools
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	for _, client := range s.Clients {
		client.Close()
	}
}
