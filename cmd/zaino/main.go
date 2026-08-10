package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/ui"
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
	)
	flag.Parse()

	backend, err := provider.New(*providerName)
	if err != nil {
		return explainCredentials(err)
	}

	ag := &agent.Agent{
		Provider:  backend,
		Model:     *model,
		MaxTokens: *maxTokens,
		System:    *system,
		Effort:    *effort,
		Thinking:  &llm.Thinking{Enabled: true, Show: *showThink},
	}

	if *plain || *verbose || !isTerminal(os.Stdin) {
		return runPlain(ag, backend.Name(), *showThink, *verbose)
	}

	program := tea.NewProgram(
		ui.New(ag, backend.Name()),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err = program.Run()
	return err
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
