package gateway

import (
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestRouteCacheTrimAndInvalidateProject(t *testing.T) {
	c := NewRouteCache()
	c.maxRoutes = 2
	c.maxProd = 2

	r1 := &registry.Route{ProjectID: "demo", VersionID: "v1", Active: true}
	r2 := &registry.Route{ProjectID: "demo", VersionID: "v2", Active: true}
	r3 := &registry.Route{ProjectID: "demo", VersionID: "v3", Active: true}
	c.SetRoute("demo", "v1", r1, true)
	c.SetRoute("demo", "v2", r2, true)
	c.SetRoute("demo", "v3", r3, true)
	if c.routeL.Len() > 2 {
		t.Fatalf("expected trim to maxRoutes=2, len=%d", c.routeL.Len())
	}

	cProd := NewRouteCache()
	cProd.maxProd = 1
	pa := "a"
	cProd.SetProd("p1", &pa, true)
	pb := "b"
	cProd.SetProd("p2", &pb, true)
	if cProd.prodL.Len() > 1 {
		t.Fatalf("expected trim prod to 1, len=%d", cProd.prodL.Len())
	}

	v1 := "v1"
	c.SetProd("other", &v1, true)

	c.SetRoute("demo", "v1", r1, true)
	prodDemo := "pd"
	c.SetProd("demo", &prodDemo, true)

	c.InvalidateProject("demo")

	if _, hit, ok := c.GetRoute("demo", "v1"); hit || ok {
		t.Fatal("expected route cache miss after InvalidateProject")
	}
	if _, hit, ok := c.GetProd("demo"); hit || ok {
		t.Fatal("expected prod cache miss after InvalidateProject")
	}
	if _, hit, ok := c.GetProd("other"); !hit || !ok {
		t.Fatal("expected other project prod still cached")
	}
}
