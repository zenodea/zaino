package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/llm"
)

const modelFetchTimeout = 20 * time.Second

type modelsMsg struct {
	provider string
	models   []string
	err      error
}

// fetchModels asks the provider what this credential actually reaches. It runs
// off the UI thread: the answer is a network round trip.
func fetchModels(p llm.Provider) tea.Cmd {
	fetcher, ok := p.(llm.ModelFetcher)
	if !ok {
		return nil
	}
	name := p.Name()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), modelFetchTimeout)
		defer cancel()

		models, err := fetcher.FetchModels(ctx)
		return modelsMsg{provider: name, models: models, err: err}
	}
}

func (m *Model) receiveModels(msg modelsMsg) tea.Cmd {
	if msg.err != nil {
		// The static list still works, so this is a notice and not an error.
		if m.awaitingModels {
			m.awaitingModels = false
			m.notice("could not list %s models: %s", msg.provider, msg.err)
		}
		return nil
	}

	if m.fetched == nil {
		m.fetched = map[string][]string{}
	}
	m.fetched[msg.provider] = msg.models

	// Only take over the screen if the user is still waiting on this list.
	if m.awaitingModels && msg.provider == m.provider {
		m.awaitingModels = false
		if len(msg.models) == 0 {
			m.notice("%s listed no models", msg.provider)
			return nil
		}
		return m.openModelChooser()
	}
	return nil
}

// knownModels prefers what the provider just told us over what was compiled in.
func (m *Model) knownModels() []string {
	if live := m.fetched[m.provider]; len(live) > 0 {
		return live
	}
	lister, ok := m.agent.Provider.(llm.ModelLister)
	if !ok {
		return nil
	}
	return lister.Models()
}
