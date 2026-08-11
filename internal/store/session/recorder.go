package session

import "github.com/zenodea/zaino/internal/llm"

type Recorder struct {
	store Store

	written int
	pending []llm.Usage
}

func NewRecorder(store Store) *Recorder { return &Recorder{store: store} }

func (r *Recorder) Store() Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Recorder) Use(store Store, written int) {
	r.store, r.written, r.pending = store, written, nil
}

func (r *Recorder) Turn(u llm.Usage) {
	if r == nil {
		return
	}
	r.pending = append(r.pending, u)
}

func (r *Recorder) Append(n New) error {
	if r == nil || r.store == nil {
		return nil
	}
	_, err := r.store.Append(n)
	return err
}

func (r *Recorder) Messages(messages []llm.Message) error {
	if r == nil {
		return nil
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
		if err := r.Append(Message(msg, usage)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.written, r.pending = len(messages), nil
	return firstErr
}

func (r *Recorder) Clear() error {
	if r == nil {
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
