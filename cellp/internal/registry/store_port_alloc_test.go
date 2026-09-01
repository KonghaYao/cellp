package registry

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func openTestPortStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenWithOptions(filepath.Join(dir, "ports.sqlite"), OpenOptions{
		IngressPortMin: 19080,
		IngressPortMax: 19095,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAllocateIngressListenPortEphemeralTwoOwners(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()

	a1, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{
		OwnerID: "owner-a", Stability: PortStabilityEphemeral,
	})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{
		OwnerID: "owner-b", Stability: PortStabilityEphemeral,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a1.Port == a2.Port {
		t.Fatalf("expected distinct ports got %d", a1.Port)
	}
	active, err := s.ListActivePortAllocations(ctx, PortPurposeIngressListen)
	if err != nil || len(active) != 2 {
		t.Fatalf("active=%d err=%v", len(active), err)
	}
}

func TestAllocateIngressListenPortConcurrent(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	const n = 12
	ports := make(chan int, n)
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[int]struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pa, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{
				OwnerID: fmt.Sprintf("owner-%d", i),
			})
			if err != nil {
				t.Error(err)
				return
			}
			ports <- pa.Port
		}(i)
	}
	wg.Wait()
	close(ports)
	for p := range ports {
		mu.Lock()
		if _, ok := seen[p]; ok {
			t.Fatalf("duplicate port %d", p)
		}
		seen[p] = struct{}{}
		mu.Unlock()
	}
	active, _ := s.ListActivePortAllocations(ctx, PortPurposeIngressListen)
	if len(active) != n {
		t.Fatalf("active count=%d want %d", len(active), n)
	}
}

func TestReserveStablePortConflict(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	const stablePort = 19090
	if stablePort < 19080 || stablePort > 19095 {
		t.Fatalf("stablePort outside test pool")
	}
	_, err := s.ReserveStablePort(ctx, ReserveStablePortInput{
		Port: stablePort, OwnerID: ProdPortReserveOwnerID("p1"), ProjectID: strPtr("p1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, pa := range mustListActive(t, s, ctx) {
		if pa.Port == stablePort && pa.OwnerID != ProdPortReserveOwnerID("p1") {
			t.Fatalf("allocate took reserved port %d", stablePort)
		}
	}
	_, err = s.ReserveStablePort(ctx, ReserveStablePortInput{
		Port: stablePort, OwnerID: ProdPortReserveOwnerID("p2"),
	})
	if !errors.Is(err, ErrPortConflict) {
		t.Fatalf("expected conflict got %v", err)
	}
}

func TestReleasePortThenReallocate(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	pa, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "rel-a"})
	if err != nil {
		t.Fatal(err)
	}
	port := pa.Port
	if err := s.ReleasePort(ctx, ReleasePortInput{AllocationID: pa.AllocationID, ReleaseReason: "test"}); err != nil {
		t.Fatal(err)
	}
	pa2, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "rel-b"})
	if err != nil {
		t.Fatal(err)
	}
	if pa2.AllocationID == pa.AllocationID {
		t.Fatal("expected new allocation")
	}
	// May reuse same port after release.
	_ = port
}

func TestAllocateIngressListenPortIdempotentOwner(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	a1, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "same"})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "same"})
	if err != nil {
		t.Fatal(err)
	}
	if a1.AllocationID != a2.AllocationID || a1.Port != a2.Port {
		t.Fatalf("idempotent: %+v vs %+v", a1, a2)
	}
}

func TestReserveStablePortIdempotent(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	in := ReserveStablePortInput{Port: 19081, OwnerID: "stable-owner"}
	r1, err := s.ReserveStablePort(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.ReserveStablePort(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r1.AllocationID != r2.AllocationID {
		t.Fatalf("idempotent reserve")
	}
}

func TestReserveStablePortSameOwnerDifferentPort(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	owner := "stable-drift"
	_, err := s.ReserveStablePort(ctx, ReserveStablePortInput{Port: 19082, OwnerID: owner})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReserveStablePort(ctx, ReserveStablePortInput{Port: 19083, OwnerID: owner})
	if !errors.Is(err, ErrPortConflict) {
		t.Fatalf("expected conflict got %v", err)
	}
}

func TestAllocateIngressListenPortPoolExhausted(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenWithOptions(filepath.Join(dir, "tiny.sqlite"), OpenOptions{
		IngressPortMin: 19080,
		IngressPortMax: 19080,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, err = s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AllocateIngressListenPort(ctx, AllocateIngressListenPortInput{OwnerID: "two"})
	if !errors.Is(err, ErrPortPoolExhausted) {
		t.Fatalf("expected exhausted got %v", err)
	}
}

func TestAttachIngressListenPortLedger(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	gw := "gw-1"
	bid := uuid.NewString()
	host := "v1.demo.ingress.local"
	err := s.AttachIngressListenPort(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth.demo.local",
		OwnerGatewayID: &gw, Active: true,
	}, AllocateIngressListenPortInput{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetIngressBinding(ctx, bid)
	if err != nil || got.ListenPort == nil {
		t.Fatalf("binding=%+v err=%v", got, err)
	}
	pa, err := s.GetActivePortAllocationByOwner(ctx, PortOwnerIngressBinding, bid)
	if err != nil || pa == nil || pa.Port != *got.ListenPort {
		t.Fatalf("ledger=%+v binding port=%v err=%v", pa, got.ListenPort, err)
	}
}

func TestDetachIngressListenPort(t *testing.T) {
	s := openTestPortStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	bid := uuid.NewString()
	host := "v1.demo.ingress.local"
	if err := s.AttachIngressListenPort(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth.demo.local", Active: true,
	}, AllocateIngressListenPortInput{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DetachIngressListenPort(ctx, bid, "archive"); err != nil {
		t.Fatal(err)
	}
	pa, _ := s.GetActivePortAllocationByOwner(ctx, PortOwnerIngressBinding, bid)
	if pa != nil {
		t.Fatalf("expected released ledger got %+v", pa)
	}
	got, _ := s.GetIngressBinding(ctx, bid)
	if got == nil || got.ListenPort != nil {
		t.Fatalf("expected listen_port cleared got %+v", got)
	}
}

func mustListActive(t *testing.T, s *SQLiteStore, ctx context.Context) []PortAllocation {
	t.Helper()
	out, err := s.ListActivePortAllocations(ctx, PortPurposeIngressListen)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
