package fanoutsub

import (
	"context"
	"errors"
	"sync"
)

var ErrSubOnRunning = errors.New("subscribing on running fanout is not allowed")
var ErrAlreadyRunning = errors.New("starting already running fanout")

type Fanout[T any] struct {
	mu sync.Mutex

	srcCh     <-chan T
	subs      []chan<- T
	isRunning bool
}

// New creates new Fanout[T].
func New[T any](srcCh <-chan T) *Fanout[T] {
	return &Fanout[T]{
		srcCh: srcCh,
	}
}

// Subscribe channel to fanout.
func (f *Fanout[T]) Subscribe(dstCh chan<- T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isRunning {
		return ErrSubOnRunning
	}

	f.subs = append(f.subs, dstCh)
	return nil
}

// Start runs fanout. Deadlocks are possible if one of the subs is not reading.
// When context is done, all subs are cleaned.
func (f *Fanout[T]) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isRunning {
		return ErrAlreadyRunning
	}
	f.isRunning = true

	go func() {
		defer func() {
			f.mu.Lock()
			f.isRunning = false
			f.subs = nil
			f.mu.Unlock()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-f.srcCh:
				if !ok {
					return
				}
				for i := range f.subs {
					f.subs[i] <- data
				}
			}
		}
	}()

	return nil
}
