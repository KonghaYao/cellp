package controllermtls

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWaitForStartupReadyOnly(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	done := make(chan struct{})
	runErr := make(chan error, 1)
	readyFn := func() <-chan struct{} { return ready }
	ctx := context.Background()
	if err := WaitForStartup(ctx, readyFn, done, runErr); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestWaitForStartupReadyWithDoneClosedAndNilRunErr(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	done := make(chan struct{})
	close(done)
	runErr := make(chan error, 1)
	runErr <- nil
	readyFn := func() <-chan struct{} { return ready }
	ctx := context.Background()
	err := WaitForStartup(ctx, readyFn, done, runErr)
	if err == nil {
		t.Fatal("expected error when ready and Run already finished")
	}
	if !strings.Contains(err.Error(), "stopped during startup") {
		t.Fatalf("expected stopped-during-startup, got %v", err)
	}
}

func TestWaitForStartupReadyWithDoneClosedAndRunErr(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	done := make(chan struct{})
	close(done)
	runErr := make(chan error, 1)
	serveFail := errors.New("serve failed")
	runErr <- serveFail
	readyFn := func() <-chan struct{} { return ready }
	err := WaitForStartup(context.Background(), readyFn, done, runErr)
	if err == nil {
		t.Fatal("expected wrapped serve error")
	}
	if !errors.Is(err, serveFail) {
		t.Fatalf("expected %v, got %v", serveFail, err)
	}
}

func TestWaitForStartupContextCanceled(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan struct{})
	runErr := make(chan error, 1)
	readyFn := func() <-chan struct{} { return ready }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitForStartup(ctx, readyFn, done, runErr); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestWaitForStartupRunErrBeforeReady(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan struct{})
	runErr := make(chan error, 1)
	bindErr := fmt.Errorf("bind: %w", errors.New("address already in use"))
	runErr <- bindErr
	readyFn := func() <-chan struct{} { return ready }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := WaitForStartup(ctx, readyFn, done, runErr)
	if err == nil {
		t.Fatal("expected bind error")
	}
	if !errors.Is(err, bindErr) {
		t.Fatalf("expected %v, got %v", bindErr, err)
	}
}
