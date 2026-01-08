package fanoutsub

import (
	"context"
	"errors"
	"sync"
)

var ErrSubOnRunning = errors.New("subscribing on running fanout is not allowed")
var ErrAlreadyRunning = errors.New("starting already running fanout")
var ErrUnsubOnRunning = errors.New("unsubscribing on running fanout is not allowed")

type Fanout[T any] struct {
	mu sync.Mutex

	srcCh     <-chan T
	subs      map[chan<- T]struct{}
	isRunning bool
}

// New creates new Fanout[T].
func New[T any](srcCh <-chan T) *Fanout[T] {
	return &Fanout[T]{
		srcCh: srcCh,
		subs:  make(map[chan<- T]struct{}),
	}
}

// Subscribe channel to fanout.
func (f *Fanout[T]) Subscribe(dstCh chan<- T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isRunning {
		return ErrSubOnRunning
	}

	f.subs[dstCh] = struct{}{}
	return nil
}

// Unsubscribe channel from fanout.
func (f *Fanout[T]) Unsubscribe(dstCh chan<- T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isRunning {
		return ErrUnsubOnRunning
	}

	delete(f.subs, dstCh)
	return nil
}

// Start runs fanout. Each message is sent to subscribers in separate goroutines to avoid blocking.
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
			clear(f.subs)
			f.mu.Unlock()
		}()

		var wg sync.WaitGroup
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-f.srcCh:
				if !ok {
					return
				}
				for dstCh := range f.subs {
					wg.Go(func() {
						dstCh <- data
					})
				}
				wg.Wait()
			}
		}
	}()

	return nil
}
