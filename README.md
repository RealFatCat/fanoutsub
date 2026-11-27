# fanoutsub

A simple Go package that implements a fanout pattern for channels. It allows distributing messages from one source channel to multiple subscriber channels.

## Usage

```go
package main

import (
    "context"
    "github.com/realfatcat/fanoutsub"
)

func main() {
    src := make(chan int)
    f := fanoutsub.New(src)

    sub1 := make(chan int, 1)
    sub2 := make(chan int, 1)

    if err := f.Subscribe(sub1); err != nil {
        panic(err)
    }
    if err := f.Subscribe(sub2); err != nil {
        panic(err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := f.Start(ctx); err != nil {
        panic(err)
    }

    src <- 42
    close(src)

    // Wait for the run to finish: Done is closed after the fanout
    // completes (source channel closed or context cancelled).
    <-f.Done()
}
```

## Features

- Thread-safe: `Subscribe`, `Unsubscribe`, `Start` and `Done` are guarded by a mutex. The running fanout iterates a snapshot of subscribers captured at `Start`, so it never races with control-plane calls.
- Uses Go generics for type safety.
- Delivers each message to subscribers captured at `Start` sequentially in source order (single goroutine for the fanout, no per-message or per-subscriber goroutines). A slow or full subscriber blocks the fanout — deadlocks are possible. The delivery order between subscribers is unspecified.
- `Start` is asynchronous: it launches the fanout in a background goroutine and returns immediately. `Done()` returns a channel that is closed when the run finishes — it is never nil, so `<-f.Done()` can be used before or after `Start`.
- On source channel close, every buffered message is delivered to all subscribers, then every subscriber channel is closed.
- On context cancellation, the fanout may abort an in-flight message partway — some subscribers may have received it, others may not. The fanout then returns and closes every subscriber channel it holds.
- Subscriber channels that were unsubscribed before `Start` are not closed by the fanout and remain caller-owned — do not range over them after the run finishes.
- Consumers must drain their channel and exit when it is closed, and must not close the subscriber channels themselves.
- Single-use lifecycle: once the run finishes, the instance cannot be reused — `Start`, `Subscribe` and `Unsubscribe` return `ErrFinished`.

## Errors

| Error | When |
| --- | --- |
| `ErrSubOnRunning` | `Subscribe` while the fanout is running |
| `ErrUnsubOnRunning` | `Unsubscribe` while the fanout is running |
| `ErrNotSubscribed` | `Unsubscribe` on a channel that is not subscribed |
| `ErrNilChannel` | `Subscribe` with a nil channel |
| `ErrAlreadyRunning` | `Start` while the fanout is running |
| `ErrFinished` | `Start` / `Subscribe` / `Unsubscribe` after the run finished |
