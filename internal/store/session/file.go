package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zenodea/zaino/internal/x/fsx"
	"github.com/zenodea/zaino/internal/x/paths"
)

type FileRepo struct {
	dir string
	cwd string
}

func Open(cwd string) (*FileRepo, error) {
	dir, err := paths.Data("sessions", paths.Slug(cwd))
	if err != nil {
		return nil, err
	}
	return OpenDir(dir, cwd)
}

func OpenDir(dir, cwd string) (*FileRepo, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileRepo{dir: dir, cwd: cwd}, nil
}

func (r *FileRepo) Dir() string { return r.dir }

func (r *FileRepo) Create() (Store, error) {
	now := time.Now()
	meta := Meta{
		Version: Version,
		ID:      now.Format("20060102-150405") + "-" + token(2),
		Started: now,
		Cwd:     r.cwd,
	}

	line, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(r.dir, meta.ID+".jsonl")
	if err := fsx.AppendLine(path, string(line)); err != nil {
		return nil, err
	}
	return openFile(path, meta, nil)
}

func (r *FileRepo) Open(id string) (Store, error) {
	path, err := r.find(id)
	if err != nil {
		return nil, err
	}
	return loadFile(path)
}

func (r *FileRepo) Latest() (Summary, bool, error) {
	all, err := r.List()
	if err != nil || len(all) == 0 {
		return Summary{}, false, err
	}
	return all[0], true, nil
}

func (r *FileRepo) List() ([]Summary, error) {
	names, err := r.files()
	if err != nil {
		return nil, err
	}

	var out []Summary
	for _, path := range names {
		s, err := summarize(path)
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

func (r *FileRepo) files() ([]string, error) {
	items, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, item := range items {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".jsonl") {
			out = append(out, filepath.Join(r.dir, item.Name()))
		}
	}
	return out, nil
}

func (r *FileRepo) find(id string) (string, error) {
	if id == "" {
		return "", ErrNotFound
	}
	names, err := r.files()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, path := range names {
		name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if name == id {
			return path, nil
		}
		if strings.HasPrefix(name, id) {
			matches = append(matches, path)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %s", ErrNotFound, id)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, path := range matches {
			ids[i] = strings.TrimSuffix(filepath.Base(path), ".jsonl")
		}
		return "", fmt.Errorf("session: %q matches %s", id, strings.Join(ids, ", "))
	}
}

type fileStore struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	meta    Meta
	entries []Entry
	leaf    string
	seq     int
}

func openFile(path string, meta Meta, entries []Entry) (*fileStore, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s := &fileStore{path: path, f: f, meta: meta, entries: entries}
	if n := len(entries); n > 0 {
		s.leaf, s.seq = entries[n-1].ID, entries[n-1].Seq
	}
	return s, nil
}

func (s *fileStore) Meta() Meta { return s.meta }

func (s *fileStore) Append(n New) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := Entry{
		ID:     token(4),
		Parent: s.leaf,
		Seq:    s.seq + 1,
		At:     time.Now().UTC().Truncate(time.Millisecond),
		Type:   n.Type,
		body:   n.body,
	}

	line, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return Entry{}, err
	}

	s.entries = append(s.entries, e)
	s.leaf, s.seq = e.ID, e.Seq
	return e, nil
}

func (s *fileStore) Entries() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...), nil
}

func (s *fileStore) Leaf() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leaf, nil
}

// The empty id is the root, which leaves everything recorded so far on a
// branch of its own.
func (s *fileStore) SetLeaf(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == "" {
		s.leaf = ""
		return nil
	}
	for _, e := range s.entries {
		if e.ID == id {
			s.leaf = id
			return nil
		}
	}
	return fmt.Errorf("%w: entry %s", ErrNotFound, id)
}

func (s *fileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func loadFile(path string) (*fileStore, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(raw), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("session %s: empty file", filepath.Base(path))
	}

	var meta Meta
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		return nil, fmt.Errorf("session %s line 1: %w", filepath.Base(path), err)
	}
	if meta.Version > Version {
		return nil, fmt.Errorf("session %s: format v%d, this zaino understands v%d",
			filepath.Base(path), meta.Version, Version)
	}

	repaired := false
	entries := make([]Entry, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		var e Entry
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			// A bad last line is an append that was cut off: drop it and
			// republish the whole part. Anywhere else is real damage.
			if i == len(lines)-1 {
				valid := strings.Join(lines[:i], "\n") + "\n"
				if err := fsx.WriteAtomic(path, []byte(valid), 0o600); err != nil {
					return nil, err
				}
				repaired = true
				break
			}
			return nil, fmt.Errorf("session %s line %d: %w", filepath.Base(path), i+1, err)
		}
		entries = append(entries, e)
	}

	if !repaired && len(raw) > 0 && raw[len(raw)-1] != '\n' {
		if err := fsx.AppendLine(path, ""); err != nil {
			return nil, err
		}
	}
	return openFile(path, meta, entries)
}

func summarize(path string) (Summary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Summary{}, err
	}
	s, err := loadFile(path)
	if err != nil {
		return Summary{}, err
	}
	defer s.Close()

	out := Summary{Meta: s.meta, Updated: info.ModTime()}
	for _, e := range Path(s.entries) {
		if e.Type != KindMessage || e.Message == nil {
			continue
		}
		out.Messages++
		if out.Preview == "" {
			out.Preview = firstLine(e.Prompt())
		}
		if e.Usage != nil {
			out.Tokens += e.Usage.InputTokens + e.Usage.OutputTokens
		}
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func token(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%0*x", n*2, time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b)
}
