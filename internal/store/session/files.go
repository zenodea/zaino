package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/zenodea/zaino/internal/x/fsx"
)

const MaxBlob = 4 << 20

type Change struct {
	Path    string
	Existed bool
	Before  []byte
	After   []byte
	Mode    fs.FileMode
}

type FileChange struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed,omitempty"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
	Large   bool   `json:"large,omitempty"`
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Blobs struct{ dir string }

func OpenBlobs(dir string) (*Blobs, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Blobs{dir: dir}, nil
}

func (b *Blobs) Put(data []byte) (string, error) {
	hash := Hash(data)
	path := filepath.Join(b.dir, hash)
	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}
	return hash, fsx.WriteAtomic(path, data, 0o600)
}

func (b *Blobs) Get(hash string) ([]byte, error) {
	return os.ReadFile(filepath.Join(b.dir, hash))
}

func (b *Blobs) Keep(c Change) (FileChange, error) {
	fc := FileChange{Path: c.Path, Existed: c.Existed, Mode: uint32(c.Mode.Perm())}
	if len(c.Before) > MaxBlob || len(c.After) > MaxBlob {
		fc.Large = true
		if c.Existed {
			fc.Before = Hash(c.Before)
		}
		fc.After = Hash(c.After)
		return fc, nil
	}
	var err error
	if c.Existed {
		if fc.Before, err = b.Put(c.Before); err != nil {
			return fc, err
		}
	}
	fc.After, err = b.Put(c.After)
	return fc, err
}

type FileState struct {
	Path         string
	Want         string
	WantExists   bool
	Expect       string
	ExpectExists bool
	Restorable   bool
}

func changesAlong(path []Entry) []FileChange {
	var out []FileChange
	for _, e := range path {
		out = append(out, e.Files...)
	}
	return out
}

func lastChange(changes []FileChange, path string) (FileChange, bool) {
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].Path == path {
			return changes[i], true
		}
	}
	return FileChange{}, false
}

func firstChange(changes []FileChange, path string) (FileChange, bool) {
	for _, c := range changes {
		if c.Path == path {
			return c, true
		}
	}
	return FileChange{}, false
}

// PlanFiles is what moving the leaf from one entry to another means for the
// files: for each one touched on either line, what it should hold at the
// destination and what it is expected to hold now.
func PlanFiles(entries []Entry, from, to string) []FileState {
	fromChanges := changesAlong(PathTo(entries, from))
	toChanges := changesAlong(PathTo(entries, to))

	touched := map[string]bool{}
	large := map[string]bool{}
	for _, c := range append(append([]FileChange(nil), fromChanges...), toChanges...) {
		touched[c.Path] = true
		if c.Large {
			large[c.Path] = true
		}
	}

	var plan []FileState
	for path := range touched {
		s := FileState{Path: path, Restorable: !large[path]}
		if c, ok := lastChange(toChanges, path); ok {
			s.Want, s.WantExists = c.After, true
		} else if c, ok := firstChange(fromChanges, path); ok {
			s.Want, s.WantExists = c.Before, c.Existed
		}
		if c, ok := lastChange(fromChanges, path); ok {
			s.Expect, s.ExpectExists = c.After, true
		} else if c, ok := firstChange(toChanges, path); ok {
			s.Expect, s.ExpectExists = c.Before, c.Existed
		}
		if s.Want == s.Expect && s.WantExists == s.ExpectExists {
			continue
		}
		plan = append(plan, s)
	}
	sort.Slice(plan, func(i, j int) bool { return plan[i].Path < plan[j].Path })
	return plan
}

func (s FileState) abs(root string) string {
	if filepath.IsAbs(s.Path) {
		return s.Path
	}
	return filepath.Join(root, s.Path)
}

func (s FileState) conflicts(root string) bool {
	data, err := os.ReadFile(s.abs(root))
	if errors.Is(err, os.ErrNotExist) {
		return s.ExpectExists
	}
	if err != nil {
		return true
	}
	return !s.ExpectExists || Hash(data) != s.Expect
}

// Conflicts are the files that are not as the tree expects: something other
// than zaino wrote them since.
func Conflicts(root string, plan []FileState) []string {
	var out []string
	for _, s := range plan {
		if s.Restorable && s.conflicts(root) {
			out = append(out, s.Path)
		}
	}
	return out
}

type Restored struct {
	Files     []string
	Conflicts []string
	Skipped   []string
}

func RestoreFiles(root string, blobs *Blobs, plan []FileState, force bool) (Restored, error) {
	var r Restored
	for _, s := range plan {
		if !s.Restorable {
			r.Skipped = append(r.Skipped, s.Path)
			continue
		}
		if !force && s.conflicts(root) {
			r.Conflicts = append(r.Conflicts, s.Path)
			continue
		}
		abs := s.abs(root)
		if !s.WantExists {
			if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
				return r, err
			}
			r.Files = append(r.Files, s.Path)
			continue
		}
		data, err := blobs.Get(s.Want)
		if err != nil {
			return r, err
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return r, err
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return r, err
		}
		r.Files = append(r.Files, s.Path)
	}
	return r, nil
}
