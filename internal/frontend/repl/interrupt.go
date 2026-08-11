package repl

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

type interrupts struct {
	ch chan os.Signal

	mu     sync.Mutex
	cancel context.CancelFunc
}

func newInterrupts() *interrupts {
	i := &interrupts{ch: make(chan os.Signal, 1)}
	signal.Notify(i.ch, os.Interrupt)
	go func() {
		for range i.ch {
			i.mu.Lock()
			cancel := i.cancel
			i.mu.Unlock()
			if cancel == nil {
				fmt.Fprintln(os.Stderr)
				os.Exit(130)
			}
			cancel()
		}
	}()
	return i
}

func (i *interrupts) bind(cancel context.CancelFunc) {
	i.mu.Lock()
	i.cancel = cancel
	i.mu.Unlock()
}

func (i *interrupts) unbind() {
	i.mu.Lock()
	i.cancel = nil
	i.mu.Unlock()
}

func (i *interrupts) stop() {
	signal.Stop(i.ch)
	close(i.ch)
}
