// Package last remembers what was picked at the prompt — /provider, /model
// and /effort — so the next zaino in the same project starts from it. The file
// is state, not config: zaino writes it, nothing in it is meant to be edited by
// hand, and where it ranks among the config files is config.Load's business.
package last

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/zenodea/zaino/internal/store/session"
	"github.com/zenodea/zaino/internal/x/fsx"
	"github.com/zenodea/zaino/internal/x/paths"
)

// Settings is what one project last ran with. A nil Model or Effort was never
// picked; an empty one was picked back to the provider's default, which is a
// choice too, and beats a config file that says otherwise.
type Settings struct {
	Provider string  `json:"provider,omitempty"`
	Model    *string `json:"model,omitempty"`
	Effort   *string `json:"effort,omitempty"`

	// The file it came from, for /config. Empty when nothing was remembered.
	From string `json:"-"`
}

func (s Settings) empty() bool {
	return s.Provider == "" && s.Model == nil && s.Effort == nil
}

func (s *Settings) merge(o Settings) {
	if o.Provider != "" {
		s.Provider = o.Provider
	}
	if o.Model != nil {
		s.Model = o.Model
	}
	if o.Effort != nil {
		s.Effort = o.Effort
	}
}

// Of says what a session entry is worth remembering: a model entry carries
// its provider too, an effort entry its level, and anything else nothing.
func Of(n session.New) (Settings, bool) {
	switch n.Type {
	case session.KindModel:
		model := n.Model
		return Settings{Provider: n.Provider, Model: &model}, true
	case session.KindEffort:
		level := n.Level
		return Settings{Effort: &level}, true
	}
	return Settings{}, false
}

type file struct {
	// Recent is the last pick anywhere: what a project with no memory of its
	// own starts from.
	Recent   Settings            `json:"recent"`
	Projects map[string]Settings `json:"projects,omitempty"`
}

// Store is the memory of one project — the directory holding .zaino or the
// repository, or "" outside any — in $XDG_STATE_HOME/zaino/last.json.
type Store struct {
	path    string
	project string
}

func Open(project string) (*Store, error) {
	path, err := paths.State("last.json")
	if err != nil {
		return nil, err
	}
	return &Store{path: path, project: project}, nil
}

func (s *Store) Path() string { return s.path }

// Load is what this project last ran with, or failing that what was picked
// most recently anywhere. Nothing yet is not an error.
func (s *Store) Load() (Settings, error) {
	f, err := s.read()
	if err != nil {
		return Settings{}, err
	}
	got, ok := f.Projects[s.project]
	if !ok || s.project == "" {
		got = f.Recent
	}
	if !got.empty() {
		got.From = s.path
	}
	return got, nil
}

// Remember folds a change into the project's entry and into the recent one.
// A project's entry starts from what Load would have given it, so from its
// first pick on the project stands on its own and later picks elsewhere do
// not reach it.
func (s *Store) Remember(change Settings) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	if s.project != "" {
		entry, ok := f.Projects[s.project]
		if !ok {
			entry = f.Recent
		}
		entry.merge(change)
		if f.Projects == nil {
			f.Projects = map[string]Settings{}
		}
		f.Projects[s.project] = entry
	}
	f.Recent.merge(change)

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(s.path, append(data, '\n'), 0o600)
}

func (s *Store) read() (file, error) {
	var f file
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("%s: %w", s.path, err)
	}
	return f, nil
}
