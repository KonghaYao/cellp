package serve

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func TestNextRuntimeNodeGeneration(t *testing.T) {
	now := time.Now().UTC()
	gen, err := nextRuntimeNodeGeneration(nil, now)
	if err != nil || gen != 1 {
		t.Fatalf("gen=%d err=%v", gen, err)
	}
	active := &contract.RuntimeNode{Generation: 3, LeaseExpiry: now.Add(time.Hour)}
	if _, err := nextRuntimeNodeGeneration(active, now); err == nil {
		t.Fatal("expected active lease error")
	}
	expired := &contract.RuntimeNode{Generation: 3, LeaseExpiry: now.Add(-time.Second)}
	gen, err = nextRuntimeNodeGeneration(expired, now)
	if err != nil || gen != 4 {
		t.Fatalf("gen=%d err=%v", gen, err)
	}
}

func TestReleaseRuntimeNodeLeaseCASViaStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	store, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	meta := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1,
		LeaseExpiry:  time.Now().UTC().Add(-time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.UpsertRuntimeNode(ctx, meta); err != nil {
		t.Fatal(err)
	}
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 2,
		LeaseExpiry:  time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.ActivateRuntimeNode(ctx, node, 1); err != nil {
		t.Fatal(err)
	}
	if err := releaseRuntimeNodeLeaseCAS(ctx, store, "n1", 1); err != registry.ErrNodeLeaseCASConflict {
		t.Fatalf("old generation must not release: %v", err)
	}
	if err := releaseRuntimeNodeLeaseCAS(ctx, store, "n1", 2); err != nil {
		t.Fatal(err)
	}
}

func TestWaitBackgroundRespectsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		time.Sleep(time.Hour)
		close(done)
	}()
	err := waitBackground(ctx, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestAbortElasticStartupDoesNotReleaseLeaseWhenNotQuiesced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	store, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	meta := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1,
		LeaseExpiry:  time.Now().UTC().Add(-time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.UpsertRuntimeNode(ctx, meta); err != nil {
		t.Fatal(err)
	}
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 2,
		LeaseExpiry:  time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.ActivateRuntimeNode(ctx, node, 1); err != nil {
		t.Fatal(err)
	}
	loopsDone := make(chan struct{})
	runDone := make(chan struct{})
	_, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	abortCtx, abortCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer abortCancel()
	if err := abortElasticStartup(abortCtx, parentCancel, store, "n1", 2, nil, runDone, loopsDone); err == nil {
		t.Fatal("expected abort shutdown error")
	}
	active, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || active == nil || active.Generation != 2 || !active.LeaseExpiry.After(time.Now().UTC()) {
		t.Fatalf("lease must remain active: node=%+v err=%v", active, err)
	}
}

func TestShutdownHTTPServerTimesOutWithBlockingHandler(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srv.Serve(ln)
	}()
	go func() {
		c := &http.Client{Timeout: 0}
		_, _ = c.Get("http://" + ln.Addr().String() + "/")
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	shCtx, shCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shCancel()
	if err := shutdownHTTPServer(shCtx, srv, srvDone); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline, got %v", err)
	}
	close(block)
}

func TestElasticAgentReleaseLeaseAfterQuiesce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	store, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	meta := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1,
		LeaseExpiry:  time.Now().UTC().Add(-time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.UpsertRuntimeNode(ctx, meta); err != nil {
		t.Fatal(err)
	}
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 2,
		LeaseExpiry:  time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.ActivateRuntimeNode(ctx, node, 1); err != nil {
		t.Fatal(err)
	}
	loopsDone := make(chan struct{})
	close(loopsDone)
	runDone := make(chan struct{})
	close(runDone)
	a := &elasticAgent{store: store, nodeID: "n1", generation: 2}
	quiesced, err := a.quiesce(context.Background())
	if err != nil || !quiesced {
		t.Fatalf("quiesce: quiesced=%v err=%v", quiesced, err)
	}
	if err := a.releaseLease(ctx); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || active == nil || active.Generation != 2 || active.LeaseExpiry.After(time.Now().UTC()) {
		t.Fatalf("lease should be released: node=%+v err=%v", active, err)
	}
}

func TestElasticAgentQuiesceTimeoutPreservesLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	store, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	meta := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1,
		LeaseExpiry:  time.Now().UTC().Add(-time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.UpsertRuntimeNode(ctx, meta); err != nil {
		t.Fatal(err)
	}
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 2,
		LeaseExpiry:  time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.ActivateRuntimeNode(ctx, node, 1); err != nil {
		t.Fatal(err)
	}
	loopsDone := make(chan struct{})
	close(loopsDone)
	runDone := make(chan struct{})
	shCtx, shCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shCancel()
	quiesced, err := quiesceElasticParts(shCtx, nil, nil, runDone, loopsDone)
	if quiesced || err == nil {
		t.Fatalf("expected quiesce failure, quiesced=%v err=%v", quiesced, err)
	}
	active, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || active == nil || !active.LeaseExpiry.After(time.Now().UTC()) {
		t.Fatalf("lease must remain active: node=%+v err=%v", active, err)
	}
}
