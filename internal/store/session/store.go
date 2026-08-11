package session

import (
	"errors"
	"time"
)

const Version = 1

var ErrNotFound = errors.New("session: not found")

type Meta struct {
	Version int       `json:"v"`
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Cwd     string    `json:"cwd"`
}

type Summary struct {
	Meta
	Updated  time.Time
	Preview  string
	Messages int
	Tokens   int
}

type Store interface {
	Meta() Meta

	Append(New) (Entry, error)

	Entries() ([]Entry, error)

	Leaf() (string, error)

	Close() error
}

type Repo interface {
	Create() (Store, error)

	Open(id string) (Store, error)

	Latest() (Summary, bool, error)

	List() ([]Summary, error)
}
