package fanoutsub

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestFanout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		sub1 := make(chan int, 10)
		sub2 := make(chan int, 10)

		if err := f.Subscribe(sub1); err != nil {
			t.Fatal(err)
		}
		if err := f.Subscribe(sub2); err != nil {
			t.Fatal(err)
		}

		ctx := t.Context()

		if err := f.Start(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		// Send some data
		src <- 1
		src <- 2
		src <- 3
		close(src)

		// Wait a bit for processing
		time.Sleep(100 * time.Millisecond)

		// Check received data
		expected := map[int]bool{1: true, 2: true, 3: true}
		for _, ch := range []<-chan int{sub1, sub2} {
			received := make(map[int]bool)
			for len(received) < 3 {
				select {
				case v := <-ch:
					received[v] = true
				case <-time.After(1 * time.Second):
					t.Fatal("timeout waiting for data")
				}
			}
			for k := range expected {
				if !received[k] {
					t.Errorf("missing value %d", k)
				}
			}
		}
	})
}

func TestSubscribeAfterStart(t *testing.T) {
	src := make(chan int)
	f := New(src)

	sub := make(chan int)
	ctx := t.Context()
	f.Start(ctx)

	if err := f.Subscribe(sub); err != ErrSubOnRunning {
		t.Errorf("expected ErrSubOnRunning, got %v", err)
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	src := make(chan int)
	f := New(src)

	ctx := t.Context()
	f.Start(ctx)

	if err := f.Start(ctx); err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestUnsubscribe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		sub1 := make(chan int, 10)
		sub2 := make(chan int, 10)

		f.Subscribe(sub1)
		f.Subscribe(sub2)

		if err := f.Unsubscribe(sub1); err != nil {
			t.Fatal(err)
		}

		ctx := t.Context()
		f.Start(ctx)

		src <- 42
		close(src)

		time.Sleep(100 * time.Millisecond)

		// sub1 should not receive, sub2 should
		select {
		case <-sub1:
			t.Error("sub1 received data after unsubscribe")
		default:
		}

		select {
		case v := <-sub2:
			if v != 42 {
				t.Errorf("expected 42, got %d", v)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("sub2 did not receive data")
		}
	})
}

func TestUnsubscribeAfterStart(t *testing.T) {
	src := make(chan int)
	f := New(src)

	sub := make(chan int)
	f.Subscribe(sub)

	// t.Context() is cancelled when the test ends, so the background
	// goroutine created by Start does not leak.
	ctx := t.Context()
	f.Start(ctx)

	if err := f.Unsubscribe(sub); err != ErrUnsubOnRunning {
		t.Errorf("expected ErrUnsubOnRunning, got %v", err)
	}
}

func TestUnsubscribeNotSubscribed(t *testing.T) {
	src := make(chan int)
	f := New(src)

	sub := make(chan int)
	if err := f.Unsubscribe(sub); err != ErrNotSubscribed {
		t.Errorf("expected ErrNotSubscribed, got %v", err)
	}
}

func TestDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		if done := f.Done(); done == nil {
			t.Error("expected non-nil Done before start")
		} else {
			select {
			case <-done:
				t.Error("Done must not be closed before start")
			default:
			}
		}

		sub := make(chan int, 10)
		if err := f.Subscribe(sub); err != nil {
			t.Fatal(err)
		}

		if err := f.Start(t.Context()); err != nil {
			t.Fatal(err)
		}

		done := f.Done()
		if done == nil {
			t.Fatal("expected non-nil Done after start")
		}

		src <- 1
		close(src)

		<-done

		// After the run finishes, the instance is finished and cannot be reused.
		if err := f.Subscribe(sub); err != ErrFinished {
			t.Errorf("expected ErrFinished, got %v", err)
		}
	})
}

func TestSubscriberChannelsClosedOnSrcClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		sub1 := make(chan int, 10)
		sub2 := make(chan int, 10)
		if err := f.Subscribe(sub1); err != nil {
			t.Fatal(err)
		}
		if err := f.Subscribe(sub2); err != nil {
			t.Fatal(err)
		}

		if err := f.Start(t.Context()); err != nil {
			t.Fatal(err)
		}

		src <- 1
		src <- 2
		close(src)

		<-f.Done()

		for _, ch := range []<-chan int{sub1, sub2} {
			var got []int
			for v := range ch {
				got = append(got, v)
			}
			if len(got) != 2 || got[0] != 1 || got[1] != 2 {
				t.Errorf("unexpected values: %v", got)
			}
		}
	})
}

func TestSubscriberChannelsClosedOnCtxCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int)
		f := New(src)

		sub := make(chan int, 10)
		if err := f.Subscribe(sub); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		if err := f.Start(ctx); err != nil {
			t.Fatal(err)
		}

		cancel()

		<-f.Done()

		if _, ok := <-sub; ok {
			t.Error("expected subscriber channel to be closed on context cancellation")
		}
	})
}

func TestStartAfterFinish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		sub := make(chan int, 10)
		if err := f.Subscribe(sub); err != nil {
			t.Fatal(err)
		}

		if err := f.Start(t.Context()); err != nil {
			t.Fatal(err)
		}

		close(src)
		<-f.Done()

		if err := f.Start(t.Context()); err != ErrFinished {
			t.Errorf("expected ErrFinished from Start, got %v", err)
		}
		if err := f.Subscribe(sub); err != ErrFinished {
			t.Errorf("expected ErrFinished from Subscribe, got %v", err)
		}
		if err := f.Unsubscribe(sub); err != ErrFinished {
			t.Errorf("expected ErrFinished from Unsubscribe, got %v", err)
		}
	})
}

func BenchmarkFanout(b *testing.B) {
	const numSubs = 4
	src := make(chan int, 1024)
	f := New(src)

	var subs []chan int
	for range numSubs {
		c := make(chan int, 1024)
		subs = append(subs, c)
		if err := f.Subscribe(c); err != nil {
			b.Fatal(err)
		}
	}

	// Drain subscribers so sends never block.
	var wg sync.WaitGroup
	for _, c := range subs {
		wg.Add(1)
		go func(c chan int) {
			defer wg.Done()
			for range c {
			}
		}(c)
	}

	ctx := context.Background()
	if err := f.Start(ctx); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		src <- 1
	}
	close(src)
	<-f.Done()
	wg.Wait()
}

func TestSubscribeNilChannel(t *testing.T) {
	src := make(chan int)
	f := New(src)

	if err := f.Subscribe(nil); err != ErrNilChannel {
		t.Errorf("expected ErrNilChannel, got %v", err)
	}
}

func TestUnsubscribedBeforeStartNotClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := make(chan int, 10)
		f := New(src)

		sub1 := make(chan int, 10)
		sub2 := make(chan int, 10)
		if err := f.Subscribe(sub1); err != nil {
			t.Fatal(err)
		}
		if err := f.Subscribe(sub2); err != nil {
			t.Fatal(err)
		}
		if err := f.Unsubscribe(sub1); err != nil {
			t.Fatal(err)
		}

		if err := f.Start(t.Context()); err != nil {
			t.Fatal(err)
		}

		src <- 1
		close(src)
		<-f.Done()

		// sub1 was unsubscribed before Start: the fanout must not close it.
		select {
		case _, ok := <-sub1:
			t.Errorf("expected sub1 to be unclosed, got ok=%v", ok)
		default:
		}

		// sub2 was subscribed and must be closed and receive the message.
		if v := <-sub2; v != 1 {
			t.Errorf("expected sub2 to receive 1, got %d", v)
		}
		if _, ok := <-sub2; ok {
			t.Error("expected sub2 to be closed")
		}
	})
}
