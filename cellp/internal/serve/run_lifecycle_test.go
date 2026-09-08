package serve

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestRunCancelStopsBackgroundWait(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(parent)
	var stopped atomic.Bool
	orchDone := make(chan struct{})
	go func() {
		<-runCtx.Done()
		stopped.Store(true)
		close(orchDone)
	}()
	runCancel()
	if err := waitBackground(context.Background(), orchDone); err != nil {
		t.Fatal(err)
	}
	if !stopped.Load() {
		t.Fatal("runCtx cancel did not stop background goroutine")
	}
	cancel()
}

func TestWaitBackgroundReturnsWhenDone(t *testing.T) {
	done := make(chan struct{})
	close(done)
	if err := waitBackground(context.Background(), done); err != nil {
		t.Fatal(err)
	}
}

func TestWaitBackgroundReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	if !errors.Is(waitBackground(ctx, done), context.Canceled) {
		t.Fatal("expected canceled")
	}
}
