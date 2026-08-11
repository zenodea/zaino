package tool

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type Workspace struct {
	Root string

	mu    sync.Mutex
	read  map[string]string
	locks map[string]*sync.Mutex
}

func NewWorkspace(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return &Workspace{Root: abs, read: map[string]string{}, locks: map[string]*sync.Mutex{}}, nil
}

type Path struct {
	Abs     string
	Rel     string
	Outside bool
}

func (p Path) String() string {
	if p.Outside {
		return p.Abs
	}
	return p.Rel
}

var errEmptyPath = errors.New("path is required")

// Symlinks are followed before the boundary is checked, so a link inside the
// workspace pointing out of it cannot be used to escape.
func (w *Workspace) Resolve(path string) (Path, error) {
	if strings.TrimSpace(path) == "" {
		return Path{}, errEmptyPath
	}

	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(w.Root, abs)
	}
	abs = filepath.Clean(abs)
	abs = resolveExisting(abs)

	rel, err := filepath.Rel(w.Root, abs)
	outside := err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if outside {
		rel = abs
	}
	return Path{Abs: abs, Rel: rel, Outside: outside}, nil
}

func resolveExisting(abs string) string {
	missing := []string{}
	at := abs
	for {
		if resolved, err := filepath.EvalSymlinks(at); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		parent := filepath.Dir(at)
		if parent == at {
			return abs
		}
		missing = append(missing, filepath.Base(at))
		at = parent
	}
}

func (w *Workspace) MarkRead(abs string, content []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.read[abs] = hash(content)
}

func (w *Workspace) WasRead(abs string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.read[abs]
	return ok
}

func (w *Workspace) Forget(abs string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.read, abs)
}

// One writer per file: two edits in the same batch would otherwise read the
// same original and the second would drop the first.
func (w *Workspace) Lock(abs string) func() {
	w.mu.Lock()
	lock, ok := w.locks[abs]
	if !ok {
		lock = &sync.Mutex{}
		w.locks[abs] = lock
	}
	w.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:8])
}
