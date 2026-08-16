// Package credentials keeps provider secrets that were entered interactively.
//
// The environment always wins: a key here is the fallback for someone who
// never exported one, not an override of what the shell already says.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"

	"github.com/zenodea/zaino/internal/x/fsx"
	"github.com/zenodea/zaino/internal/x/paths"
)

const FileName = "credentials.json"

// Perm is deliberately owner-only: the file holds API keys in the clear.
const Perm fs.FileMode = 0o600

// Method is how a provider authenticates.
type Method string

const (
	// APIKey is a static key stored in the file.
	APIKey Method = "api-key"

	// OAuth defers to the ant CLI, which holds the tokens and refreshes them.
	OAuth Method = "oauth"
)

type Entry struct {
	Method Method `json:"method"`
	Key    string `json:"key,omitempty"`

	// Profile names an ant profile for OAuth; empty means the active one.
	Profile string `json:"profile,omitempty"`
}

type file struct {
	Providers map[string]Entry `json:"providers"`
}

type Store struct {
	path string

	mu      sync.Mutex
	loaded  bool
	entries map[string]Entry
}

// Open points a store at the default location without reading it yet.
func Open() (*Store, error) {
	path, err := paths.Data(FileName)
	if err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

func At(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

func (s *Store) load() error {
	if s.loaded {
		return nil
	}
	s.entries = map[string]Entry{}
	s.loaded = true

	data, err := os.ReadFile(s.path)
	if err != nil {
		// Nothing stored yet is the normal case, not a failure.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("credentials: %w", err)
	}
	var parsed file
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("credentials: %s is not readable: %w", s.path, err)
	}
	for name, entry := range parsed.Providers {
		s.entries[name] = entry
	}
	return nil
}

// Lookup returns what is stored for a provider, if anything.
func (s *Store) Lookup(provider string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return Entry{}, false, err
	}
	entry, ok := s.entries[provider]
	return entry, ok, nil
}

func (s *Store) Set(provider string, entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	s.entries[provider] = entry
	return s.save()
}

func (s *Store) SetKey(provider, key string) error {
	return s.Set(provider, Entry{Method: APIKey, Key: key})
}

func (s *Store) SetOAuth(provider, profile string) error {
	return s.Set(provider, Entry{Method: OAuth, Profile: profile})
}

func (s *Store) Remove(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	if _, ok := s.entries[provider]; !ok {
		return nil
	}
	delete(s.entries, provider)
	return s.save()
}

// Providers lists what has been stored, in a stable order.
func (s *Store) Providers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return nil
	}
	out := make([]string, 0, len(s.entries))
	for name := range s.entries {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(file{Providers: s.entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: %w", err)
	}
	if err := fsx.WriteAtomic(s.path, append(data, '\n'), Perm); err != nil {
		return fmt.Errorf("credentials: writing %s: %w", s.path, err)
	}
	// WriteAtomic renames over any existing file, whose mode it keeps.
	return os.Chmod(s.path, Perm)
}
