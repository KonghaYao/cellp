package orch

import (
	"context"
	"testing"
	"time"
)

func TestRunExitsOnContextCancel(t *testing.T) {
	o, _, ctx := newTestOrch(t)
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		o.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}
}
