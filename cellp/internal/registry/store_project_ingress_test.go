package registry

import (
	"context"
	"testing"
)

func TestCreateProjectProdListenPortReserve(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := OpenWithOptions(dir+"/p.sqlite", OpenOptions{IngressPortMin: 19080, IngressPortMax: 19085})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	port := 19081
	p, err := s.CreateProject(ctx, CreateProjectInput{
		ID:             "demo",
		ProdListenPort: &port,
	})
	if err != nil || p == nil {
		t.Fatalf("create: %v p=%v", err, p)
	}
	pa, err := s.GetActivePortAllocationByOwner(ctx, PortOwnerIngressBinding, ProdPortReserveOwnerID("demo"))
	if err != nil || pa == nil || pa.Port != port {
		t.Fatalf("reserve: err=%v pa=%+v", err, pa)
	}
}

func TestAdoptStableIngressPortForBinding(t *testing.T) {
	ctx := context.Background()
	s, err := OpenWithOptions(t.TempDir()+"/adopt.sqlite", OpenOptions{IngressPortMin: 19080, IngressPortMax: 19085})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})

	port := 19082
	pid := "demo"
	_, err = s.ReserveStablePort(ctx, ReserveStablePortInput{
		Port: port, OwnerID: ProdPortReserveOwnerID("demo"), ProjectID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}

	bid := "demo-prod"
	host := "demo.ingress.local"
	b := IngressBinding{
		BindingID: bid, ProjectID: "demo", Role: IngressRoleProd,
		Host: &host, SyntheticHost: "synthetic.demo.ingress.local", Active: true,
	}
	if err := s.AdoptStableIngressPortForBinding(ctx, b, ProdPortReserveOwnerID("demo")); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetIngressBinding(ctx, bid)
	if err != nil || got == nil || got.ListenPort == nil || *got.ListenPort != port {
		t.Fatalf("binding: %+v err=%v", got, err)
	}
	pa, _ := s.GetActivePortAllocationByOwner(ctx, PortOwnerIngressBinding, bid)
	if pa == nil || pa.Port != port {
		t.Fatalf("owner migrated: %+v", pa)
	}
}
