# fanoutsub

A simple Go package that implements a fanout pattern for channels. It allows distributing messages from one source channel to multiple subscriber channels concurrently.

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

    f.Subscribe(sub1)
    f.Subscribe(sub2)

    ctx := context.Background()
    f.Start(ctx)

    src <- 42
    close(src)

    // Both sub1 and sub2 will receive 42
}
```

## Features

- Thread-safe with mutex protection
- Uses Go generics for type safety
- Sends messages to subscribers in separate goroutines, but deadlocks are still possible, as we wait all subscribers to read.