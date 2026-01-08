package fanoutsub

import (
	"context"
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

	ctx := context.Background()
	f.Start(ctx)

	if err := f.Unsubscribe(sub); err != ErrUnsubOnRunning {
		t.Errorf("expected ErrUnsubOnRunning, got %v", err)
	}
}
