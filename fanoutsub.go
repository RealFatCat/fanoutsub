// A simple Go package that implements a fanout pattern for channels.
// It allows distributing messages from one source channel to multiple subscriber channels.
package fanoutsub

import (
	"context"
	"errors"
	"sync"
)

var ErrSubOnRunning = errors.New("subscribing on running fanout is not allowed")
var ErrAlreadyRunning = errors.New("starting already running fanout")
var ErrUnsubOnRunning = errors.New("unsubscribing on running fanout is not allowed")
var ErrNotSubscribed = errors.New("channel is not subscribed")
var ErrNilChannel = errors.New("subscriber channel is nil")
var ErrFinished = errors.New("fanout has already finished")

type Fanout[T any] struct {
	mu        sync.Mutex
	srcCh     <-chan T
	subs      map[chan<- T]struct{}
	isRunning bool
	finished  bool
	done      chan struct{}
}

// New creates a new Fanout instance with the given source channel.
func New[T any](srcCh <-chan T) *Fanout[T] {
	return &Fanout[T]{
		srcCh: srcCh,
		subs:  make(map[chan<- T]struct{}),
		done:  make(chan struct{}),
	}
}

// Subscribe adds a subscriber channel to the fanout.
// Returns ErrNilChannel if the channel is nil,
// ErrFinished if the fanout already finished,
// ErrSubOnRunning if the fanout is running.
func (f *Fanout[T]) Subscribe(dstCh chan<- T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if dstCh == nil {
		return ErrNilChannel
	}
	if f.finished {
		return ErrFinished
	}
	if f.isRunning {
		return ErrSubOnRunning
	}

	f.subs[dstCh] = struct{}{}
	return nil
}

// Unsubscribe removes a subscriber channel from the fanout.
// Returns ErrFinished if the fanout already finished,
// ErrUnsubOnRunning if the fanout is running,
// ErrNotSubscribed if the channel is not subscribed.
func (f *Fanout[T]) Unsubscribe(dstCh chan<- T) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.finished {
		return ErrFinished
	}
	if f.isRunning {
		return ErrUnsubOnRunning
	}

	if _, ok := f.subs[dstCh]; !ok {
		return ErrNotSubscribed
	}

	delete(f.subs, dstCh)
	return nil
}

// Done returns a channel that is closed once the run finishes
// (after the context is cancelled or the source channel closes).
// It is never nil: before Start is called it is open, after the run
// finishes it is closed.
func (f *Fanout[T]) Done() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.done
}

// Start launches the fanout in a background goroutine and returns nil
// immediately; it does not block. Use Done to wait for the run to finish.
//
// Each message is delivered to all subscribers captured at Start, sequentially
// in source order — the run waits for a message to be accepted by a subscriber
// before reading the next message. A slow or full subscriber therefore blocks
// the fanout — deadlocks are possible. The delivery order between subscribers
// is unspecified.
//
// On source channel close, every buffered message is delivered to all
// subscribers and then every subscriber channel is closed.
// On context cancellation, the fanout may abort an in-flight message partway:
// some subscribers may have received it, others may not. The fanout then
// returns and closes every subscriber channel it holds.
//
// Subscriber channels that were unsubscribed before Start are not closed by
// the fanout and remain caller-owned. Consumers must drain their channel and
// exit on it being closed; consumers must not close the subscriber channels
// themselves.
//
// A Fanout instance is single-use: once the run finishes, Start, Subscribe
// and Unsubscribe return ErrFinished.
func (f *Fanout[T]) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.finished {
		return ErrFinished
	}
	if f.isRunning {
		return ErrAlreadyRunning
	}
	f.isRunning = true

	// Snapshot the subscribers once, so the running goroutine never
	// races with the map cleanup or with control-plane calls.
	subs := make([]chan<- T, 0, len(f.subs))
	for dstCh := range f.subs {
		subs = append(subs, dstCh)
	}

	go func() {
		defer func() {
			f.mu.Lock()
			f.isRunning = false
			f.finished = true
			for _, dstCh := range subs {
				close(dstCh)
			}
			clear(f.subs)
			f.mu.Unlock()
			close(f.done)
		}()

		for {
			// Check cancellation first, so that a pending source event
			// with an already-cancelled context is handled deterministically.
			if ctx.Err() != nil {
				return
			}

			select {
			case <-ctx.Done():
				return
			case data, ok := <-f.srcCh:
				if !ok {
					return
				}
				for _, dstCh := range subs {
					select {
					case <-ctx.Done():
						return
					case dstCh <- data:
					}
				}
			}
		}
	}()

	return nil
}
