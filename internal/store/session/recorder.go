package session

import (
	"errors"
	"strings"
	"sync"

	"github.com/zenodea/zaino/internal/llm"
)

type Recorder struct {
	store Store

	// A session that has nothing in it is not worth a file. Until the first
	// message lands, entries wait here and create is what turns them into one.
	create func() (Store, error)
	queued []New

	written int
	pending []llm.Usage

	blobs   *Blobs
	mu      sync.Mutex
	changes map[string][]FileChange
}

func (r *Recorder) UseBlobs(b *Blobs) { r.blobs = b }

func (r *Recorder) NoteChange(callID string, c Change) error {
	if r == nil || r.blobs == nil {
		return nil
	}
	fc, err := r.blobs.Keep(c)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.changes == nil {
		r.changes = map[string][]FileChange{}
	}
	r.changes[callID] = append(r.changes[callID], fc)
	return nil
}

func (r *Recorder) take(msg llm.Message) []FileChange {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []FileChange
	for _, block := range msg.Content {
		if result, ok := block.(llm.ToolResultBlock); ok {
			out = append(out, r.changes[result.ToolUseID]...)
			delete(r.changes, result.ToolUseID)
		}
	}
	return out
}

func NewRecorder(store Store) *Recorder { return &Recorder{store: store} }

func Lazy(create func() (Store, error)) *Recorder { return &Recorder{create: create} }

func (r *Recorder) Store() Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Recorder) Use(store Store, written int) {
	r.store, r.written, r.pending = store, written, nil
	r.create, r.queued = nil, nil
}

func (r *Recorder) open() error {
	if r.store != nil || r.create == nil {
		return nil
	}
	store, err := r.create()
	if err != nil {
		return err
	}
	r.store = store
	queued := r.queued
	r.queued = nil
	for _, n := range queued {
		if _, err := store.Append(n); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) Turn(u llm.Usage) {
	if r == nil {
		return
	}
	r.pending = append(r.pending, u)
}

func (r *Recorder) Append(n New) error {
	if r == nil {
		return nil
	}
	if r.store == nil {
		if r.create != nil {
			r.queued = append(r.queued, n)
		}
		return nil
	}
	_, err := r.store.Append(n)
	return err
}

func (r *Recorder) Messages(messages []llm.Message) error {
	if r == nil {
		return nil
	}
	if len(messages) > r.written {
		if err := r.open(); err != nil {
			return err
		}
	}
	if r.store == nil {
		r.written, r.pending = len(messages), nil
		return nil
	}

	var firstErr error
	for _, msg := range messages[min(r.written, len(messages)):] {
		var usage *llm.Usage
		if msg.Role == llm.RoleAssistant && len(r.pending) > 0 {
			u := r.pending[0]
			r.pending = r.pending[1:]
			usage = &u
		}
		if err := r.Append(MessageWith(msg, usage, r.take(msg))); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.written, r.pending = len(messages), nil
	return firstErr
}

// The kept messages are written again after the summary rather than pointed
// at where they already are. It costs a copy of the recent window per
// compaction and makes reading the log back a matter of starting at the last
// boundary and going forwards.
func (r *Recorder) Compact(summary string, kept []llm.Message) error {
	if r == nil {
		return nil
	}
	err := r.Append(Compacted(summary))
	r.written, r.pending = 0, nil

	// The summary itself is rebuilt from the entry, not stored as a message.
	if len(kept) > 0 && isSummary(kept[0]) {
		kept = kept[1:]
		r.written = 1
	}
	if messagesErr := r.Messages(kept); err == nil {
		err = messagesErr
	}
	r.written = len(kept)
	return err
}

func isSummary(m llm.Message) bool {
	return m.Role == llm.RoleUser && strings.HasPrefix(m.Text(), SummaryPrefix)
}

// Rewind takes the conversation back to its first keep messages and hangs
// what comes next off that turn. Nothing is deleted: the turns left behind
// stay in the file, on a branch of their own.
func (r *Recorder) Rewind(keep int) error {
	if r == nil {
		return nil
	}
	r.pending = nil
	if r.store == nil {
		r.written = keep
		return nil
	}

	entries, err := r.store.Entries()
	if err != nil {
		return err
	}

	marks := Build(entries).Marks
	if keep >= len(marks) {
		return nil
	}
	id := marks[keep]
	if id == "" {
		return errors.New("session: a folded-up conversation starts at its summary")
	}

	parent := ""
	for _, e := range entries {
		if e.ID == id {
			parent = e.Parent
			break
		}
	}
	if err := r.store.SetLeaf(parent); err != nil {
		return err
	}
	r.written = keep
	return nil
}

// Jump moves recording to any entry in the tree — where Rewind walks back
// along the current path, Jump crosses to a branch that was left behind.
// written is how many messages the caller has rebuilt on the way there, so
// the next Messages call appends only what is new.
func (r *Recorder) Jump(leaf string, written int) error {
	if r == nil {
		return nil
	}
	r.pending = nil
	if r.store == nil {
		r.written = written
		return nil
	}
	if err := r.store.SetLeaf(leaf); err != nil {
		return err
	}
	r.written = written
	return nil
}

func (r *Recorder) Clear() error {
	if r == nil {
		return nil
	}
	if r.store == nil {
		r.written, r.pending = 0, nil
		return nil
	}
	err := r.Append(Clear())
	r.written, r.pending = 0, nil
	return err
}

func (r *Recorder) ID() string {
	if r == nil || r.store == nil {
		return ""
	}
	return r.store.Meta().ID
}

func (r *Recorder) Close() error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Close()
}
