package activator_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
)

type fakeEnsure struct {
	calls atomic.Int32
	err   error
}

func (f *fakeEnsure) EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error {
	f.calls.Add(1)
	return f.err
}

func TestClassifyRequest(t *testing.T) {
	max := int64(1024)
	if activator.ClassifyRequest(httptest.NewRequest(http.MethodGet, "/", nil), max) != activator.WaitClassBounded {
		t.Fatal("GET should be bounded")
	}
	if activator.ClassifyRequest(httptest.NewRequest(http.MethodHead, "/", nil), max) != activator.WaitClassBounded {
		t.Fatal("HEAD should be bounded")
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = 2048
	if activator.ClassifyRequest(req, max) != activator.WaitClassFastFail {
		t.Fatal("large POST should fast-fail")
	}
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.ContentLength = 100
	if activator.ClassifyRequest(req2, max) != activator.WaitClassBounded {
		t.Fatal("small POST should be bounded")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Header.Set("Connection", "Upgrade")
	req3.Header.Set("Upgrade", "websocket")
	if activator.ClassifyRequest(req3, max) != activator.WaitClassFastFail {
		t.Fatal("websocket should fast-fail")
	}
}

func TestBudgetGlobalLimit(t *testing.T) {
	b := activator.NewBudget(1, 10)
	if !b.TryAcquire("p", "v") {
		t.Fatal("first acquire")
	}
	if b.TryAcquire("p", "v2") {
		t.Fatal("global cap")
	}
	b.Release("p", "v")
	if !b.TryAcquire("p2", "v3") {
		t.Fatal("after release")
	}
}

func TestWriteRetryResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	activator.WriteRetryResponse(rr, activator.AdmitResult{Reason: activator.ReasonWakeTimeout, RetryAfterSec: 2})
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "2" {
		t.Fatalf("retry-after %q", rr.Header().Get("Retry-After"))
	}
	if rr.Header().Get(activator.HeaderCellpReason) != activator.ReasonWakeTimeout {
		t.Fatalf("reason %q", rr.Header().Get(activator.HeaderCellpReason))
	}
}

func TestAdmitDisabledAllows(t *testing.T) {
	a := activator.New(false, &fakeEnsure{}, activator.DefaultConfig())
	res := a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 0, nil)
	if !res.AllowProxy {
		t.Fatal("disabled should allow")
	}
}

func TestAdmitFastFailTriggersEnsure(t *testing.T) {
	fe := &fakeEnsure{}
	cfg := activator.DefaultConfig()
	a := activator.New(true, fe, cfg)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = cfg.MaxBufferedBodyBytes + 1
	res := a.Admit(context.Background(), req, "p", "v", contract.StatusDeployReady, 1, func() (string, bool) { return "", false })
	if res.AllowProxy {
		t.Fatal("expected 503 path")
	}
	if res.Reason != activator.ReasonWakeRetry {
		t.Fatalf("reason %q", res.Reason)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fe.calls.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ensure not called, calls=%d", fe.calls.Load())
}

func TestAdmitBoundedTimeout(t *testing.T) {
	fe := &fakeEnsure{}
	cfg := activator.DefaultConfig()
	cfg.WakeTimeout = 50 * time.Millisecond
	cfg.PollInterval = 5 * time.Millisecond
	a := activator.New(true, fe, cfg)
	res := a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 1,
		func() (string, bool) { return "", false })
	if res.AllowProxy {
		t.Fatal("expected timeout block")
	}
	if res.Reason != activator.ReasonWakeTimeout {
		t.Fatalf("reason %q", res.Reason)
	}
	if fe.calls.Load() < 1 {
		t.Fatal("ensure should run once")
	}
}

func TestSingleflightConcurrentEnsure(t *testing.T) {
	fe := &fakeEnsure{}
	cfg := activator.DefaultConfig()
	cfg.WakeTimeout = 200 * time.Millisecond
	cfg.PollInterval = 5 * time.Millisecond
	a := activator.New(true, fe, cfg)
	var warmed atomic.Bool
	lookup := func() (string, bool) {
		if warmed.Load() {
			return "127.0.0.1:1", true
		}
		return "", false
	}
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 42, lookup)
		}()
	}
	time.Sleep(30 * time.Millisecond)
	warmed.Store(true)
	wg.Wait()
	if c := fe.calls.Load(); c != 1 {
		t.Fatalf("expected 1 ensure call, got %d", c)
	}
}

func TestAdmitArchivedRejected(t *testing.T) {
	a := activator.New(true, &fakeEnsure{}, activator.DefaultConfig())
	res := a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusArchived, 0, nil)
	if res.AllowProxy || res.Reason != activator.ReasonVersionArchived {
		t.Fatalf("got %+v", res)
	}
}

type blockingEnsure struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (f *blockingEnsure) EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error {
	if f.calls.Add(1) == 1 {
		close(f.started)
	}
	select {
	case <-f.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestCallerCancelDoesNotCancelSharedActivation(t *testing.T) {
	fe := &blockingEnsure{started: make(chan struct{}), release: make(chan struct{})}
	cfg := activator.DefaultConfig()
	cfg.WakeTimeout = time.Second
	cfg.PollInterval = time.Millisecond
	a := activator.New(true, fe, cfg)
	var warm atomic.Bool
	lookup := func() (string, bool) {
		if warm.Load() {
			return "127.0.0.1:9001", true
		}
		return "", false
	}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan activator.AdmitResult, 1)
	go func() {
		leaderDone <- a.Admit(leaderCtx, httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 1, lookup)
	}()
	<-fe.started
	followerDone := make(chan activator.AdmitResult, 1)
	go func() {
		followerDone <- a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 1, lookup)
	}()
	cancelLeader()
	if res := <-leaderDone; res.AllowProxy || res.Reason != activator.ReasonWakeTimeout {
		t.Fatalf("cancelled leader: %+v", res)
	}
	warm.Store(true)
	close(fe.release)
	if res := <-followerDone; !res.AllowProxy || res.Upstream != "127.0.0.1:9001" {
		t.Fatalf("follower lost shared activation: %+v", res)
	}
	if calls := fe.calls.Load(); calls != 1 {
		t.Fatalf("ensure calls=%d", calls)
	}
}

func TestBudgetByteLimitAndRelease(t *testing.T) {
	b := activator.NewBudgetWithBytes(2, 2, 8, 4)
	if !b.TryAcquireBytes("p", "v", 4) {
		t.Fatal("first byte reservation")
	}
	if b.TryAcquireBytes("p", "v", 1) {
		t.Fatal("per-version bytes exceeded")
	}
	b.ReleaseBytes("p", "v", 4)
	if !b.TryAcquireBytes("p", "v", 1) {
		t.Fatal("reservation not released")
	}
}

func TestConfigValidationFailClosed(t *testing.T) {
	cfg := activator.DefaultConfig()
	cfg.PerVersionPendingBytes = cfg.MaxBufferedBodyBytes - 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid byte budget")
	}
	t.Setenv("CELLP_WAKE_TIMEOUT", "not-a-duration")
	if _, err := activator.ConfigFromEnv(); err == nil {
		t.Fatal("invalid environment must fail closed")
	}
}

func TestShutdownCancelsAndQuiescesFastFailFlight(t *testing.T) {
	fe := &blockingEnsure{started: make(chan struct{}), release: make(chan struct{})}
	cfg := activator.DefaultConfig()
	a := activator.New(true, fe, cfg)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = cfg.MaxBufferedBodyBytes + 1
	_ = a.Admit(context.Background(), req, "p", "v", contract.StatusDeployReady, 1, func() (string, bool) { return "", false })
	<-fe.started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}
