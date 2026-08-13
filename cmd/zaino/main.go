package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/frontend/repl"
	"github.com/zenodea/zaino/internal/frontend/tui"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/mcp"
	"github.com/zenodea/zaino/internal/permission"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/recall"
	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/store/wirelog"
	"github.com/zenodea/zaino/internal/tool"
	"github.com/zenodea/zaino/internal/x/httpx"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "zaino:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		providerName = flag.String("provider", "auto",
			"model provider: auto|"+strings.Join(provider.Available(), "|"))
		model     = flag.String("model", "", "model id (default: the provider's own)")
		maxTokens = flag.Int("max-tokens", agent.DefaultMaxTokens, "max output tokens per turn")
		effort    = flag.String("effort", "", "output effort: low|medium|high|xhigh|max (Anthropic only)")
		system    = flag.String("system", "", "system prompt")
		showThink = flag.Bool("thinking", false, "request and display model reasoning")
		plain     = flag.Bool("plain", false, "line-based REPL instead of the full-screen UI")
		verbose   = flag.Bool("v", false, "print per-turn token usage (implies -plain)")

		permMode = flag.String("permission", string(permission.Manual),
			"when tools stop to ask: "+strings.Join(permission.ModeNames(), "|"))
		allowOutside = flag.Bool("allow-outside", false,
			"let tools reach outside the working directory")
		toolNames    = flag.String("tools", "", "give the model only these tools (comma separated)")
		excludeTools = flag.String("exclude-tools", "", "withhold these tools")
		noTools      = flag.Bool("no-tools", false, "give the model no tools at all")
		noTask       = flag.Bool("no-subagents", false, "withhold the task tool")
		mcpConfig    = flag.String("mcp", "", "MCP servers to connect to (default: mcp.json beside the sessions)")
		noMCP        = flag.Bool("no-mcp", false, "do not connect to any MCP server")

		vimKeys = flag.Bool("vim", true, "modal editing in the composer; -vim=false for plain input")
		mouse   = flag.Bool("mouse", false,
			"scroll with the wheel, at the cost of selecting text with the mouse")
		animate = flag.Bool("animate", true, "ease the transcript when ⌃j/⌃k move through it")

		carryOn  = flag.Bool("continue", false, "carry on the newest session for this directory")
		resumeID = flag.String("resume", "", "carry on a session by id, or any prefix of one")
		noSave   = flag.Bool("no-save", false, "do not record this conversation")
		logPath  = flag.String("log", "", "record what goes to the provider to a file")
	)
	flag.BoolVar(carryOn, "c", false, "shorthand for -continue")
	flag.StringVar(resumeID, "r", "", "shorthand for -resume")
	flag.Parse()

	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })

	var wire *wirelog.Log
	if *logPath != "" {
		var err error
		if wire, err = wirelog.Open(*logPath); err != nil {
			return err
		}
		defer wire.Close()
	}

	repo, store, err := openSession(*noSave, *resumeID, *carryOn)
	if err != nil {
		return err
	}
	rec := session.NewRecorder(store)
	defer rec.Close()

	var restored session.Context
	if store != nil {
		entries, err := store.Entries()
		if err != nil {
			return err
		}
		restored = session.Build(entries)
	}

	wanted := *providerName
	if !given["provider"] && !given["r"] && restored.Provider != "" {
		wanted = restored.Provider
	}
	backend, err := newProvider(wanted, wire)
	if err != nil && wanted != *providerName {
		// The session's provider is not set up here; carry on with the one that is.
		fmt.Fprintf(os.Stderr, "zaino: session used %s: %v\n", wanted, err)
		backend, err = newProvider(*providerName, wire)
	}
	if err != nil {
		return explainCredentials(err)
	}

	gate, tools, err := openToolbox(*permMode, *allowOutside, *noTools, *toolNames, *excludeTools)
	if err != nil {
		return err
	}

	servers, err := openMCP(*noMCP, *mcpConfig)
	if err != nil {
		return err
	}
	defer servers.Close()
	tools = append(tools, servers.Tools...)

	ag := &agent.Agent{
		Provider:  backend,
		Model:     *model,
		MaxTokens: *maxTokens,
		System:    *system,
		Effort:    *effort,
		Thinking:  &llm.Thinking{Enabled: true, Show: *showThink},
		Tools:     tools,
		Gate:      gate,
	}
	if !*noTools && !*noTask {
		ag.Tools = append(ag.Tools, agent.TaskTool(ag))
	}
	applyRestored(ag, restored, given)

	if restored.Provider != "" && restored.Provider != backend.Name() {
		var dropped int
		restored.Messages, dropped = session.StripProviderBlocks(restored.Messages)
		if dropped > 0 {
			fmt.Fprintf(os.Stderr,
				"zaino: dropped %d reasoning blocks only %s can read back\n", dropped, restored.Provider)
		}
	}
	if err := recordSettings(rec, ag, backend.Name(), restored); err != nil {
		fmt.Fprintln(os.Stderr, "zaino: session not being saved:", err)
	}

	if *plain || *verbose || !isTerminal(os.Stdin) {
		return repl.Run(ag, repl.Options{
			Provider:     backend.Name(),
			ShowThinking: *showThink,
			Verbose:      *verbose,
			Interactive:  isTerminal(os.Stdin),
			Gate:         gate,
			Repo:         repo,
			Recorder:     rec,
			Restored:     restored,
			Wire:         wire,
		})
	}

	m := tui.New(ag, backend.Name())
	gate.Approver = m.Approver()
	m.UseVim(*vimKeys)
	m.UseAnimation(*animate)
	if list, err := recall.Open(); err == nil {
		m.UseRecall(list)
	} else {
		fmt.Fprintln(os.Stderr, "zaino: no prompt recall:", err)
		m.UseRecall(recall.New())
	}
	m.UseSession(repo, rec)
	m.UseWireLog(wire)
	if len(restored.Messages) > 0 {
		m.Restore(restored)
	}

	options := []tea.ProgramOption{tea.WithAltScreen()}
	// Grabbing the mouse takes selection away from the terminal, so it is off by
	// default; without it the wheel arrives as ↑/↓ and lands on prompt recall.
	if *mouse {
		options = append(options, tea.WithMouseCellMotion())
	}

	program := tea.NewProgram(m, options...)
	_, err = program.Run()
	return err
}

func openToolbox(mode string, allowOutside, noTools bool, allow, deny string) (*permission.Gate, []tool.Tool, error) {
	parsed, err := permission.ParseMode(mode)
	if err != nil {
		return nil, nil, err
	}
	policy := permission.NewPolicy(parsed)
	policy.AllowOutside = allowOutside
	gate := &permission.Gate{Policy: policy}

	if noTools {
		return gate, nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tool.NewWorkspace(cwd)
	if err != nil {
		return nil, nil, err
	}
	tools, err := tool.Select(tool.All(workspace), commaList(allow), commaList(deny))
	if err != nil {
		return nil, nil, err
	}
	return gate, tools, nil
}

// A server that will not start is worth saying so about, but not worth
// refusing to run over.
func openMCP(off bool, configPath string) (*mcp.Session, error) {
	if off {
		return nil, nil
	}
	if configPath == "" {
		path, err := mcp.ConfigPath()
		if err != nil {
			return nil, nil
		}
		configPath = path
	}

	cfg, err := mcp.Load(configPath)
	if err != nil {
		return nil, err
	}
	if len(cfg.Servers) == 0 {
		return nil, nil
	}

	session, failures := mcp.Connect(context.Background(), cfg)
	for _, failure := range failures {
		fmt.Fprintln(os.Stderr, "zaino: mcp:", failure)
	}
	return session, nil
}

func commaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func openSession(noSave bool, resumeID string, carryOn bool) (session.Repo, session.Store, error) {
	if noSave {
		return nil, nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	repo, err := session.Open(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zaino: sessions are not being saved:", err)
		return nil, nil, nil
	}

	switch {
	case resumeID != "":
		store, err := repo.Open(resumeID)
		return repo, store, err

	case carryOn:
		latest, ok, err := repo.Latest()
		if err != nil {
			return nil, nil, err
		}
		if ok {
			store, err := repo.Open(latest.ID)
			return repo, store, err
		}
		fmt.Fprintln(os.Stderr, "zaino: nothing to continue here, starting fresh")
	}

	store, err := repo.Create()
	return repo, store, err
}

func applyRestored(ag *agent.Agent, c session.Context, given map[string]bool) {
	if !given["model"] && c.Model != "" {
		ag.Model = c.Model
	}
	if !given["system"] && c.System != "" {
		ag.System = c.System
	}
	if !given["effort"] && c.Effort != "" {
		ag.Effort = c.Effort
	}
	if !given["thinking"] && c.Thinking != nil {
		ag.Thinking.Show = *c.Thinking
	}
}

func recordSettings(rec *session.Recorder, ag *agent.Agent, providerName string, c session.Context) error {
	if rec.Store() == nil {
		return nil
	}

	if c.Provider != providerName || c.Model != ag.Model {
		if err := rec.Append(session.Model(providerName, ag.Model)); err != nil {
			return err
		}
	}
	if c.System != ag.System {
		if err := rec.Append(session.System(ag.System)); err != nil {
			return err
		}
	}
	if c.Effort != ag.Effort {
		if err := rec.Append(session.Effort(ag.Effort)); err != nil {
			return err
		}
	}
	if show := ag.Thinking != nil && ag.Thinking.Show; c.Thinking == nil || *c.Thinking != show {
		if err := rec.Append(session.Thinking(show)); err != nil {
			return err
		}
	}
	return nil
}

func newProvider(name string, wire *wirelog.Log) (llm.Provider, error) {
	if wire == nil {
		return provider.New(name)
	}
	client := &http.Client{Transport: wire.Transport(httpx.NewTransport())}
	return provider.New(name, provider.WithHTTPClient(client))
}

func explainCredentials(err error) error {
	if !errors.Is(err, provider.ErrNoCredentials) {
		return err
	}
	return fmt.Errorf(`%w

  Anthropic:  export ANTHROPIC_API_KEY=sk-ant-...     (console.anthropic.com)
  Gemini:     export GEMINI_API_KEY=...               (aistudio.google.com/apikey)

Then pick one with -provider, or leave it on auto`, err)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
