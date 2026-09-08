package controllermtls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/registry"
)

type guardOK struct{}

func (guardOK) EnsureActiveController(context.Context) error { return nil }

func TestCombinedMuxDispatchesVersionedPrefixes(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/mux.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	nodeURI := "spiffe://cellp/test/node/n1"
	ndHandler := &nodereg.Handler{
		Store: store,
		Allowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: nodeURI, AgentBaseURL: "https://127.0.0.1:19444", AllowLoopback: true},
		},
		Guard: guardOK{},
	}
	ndSrv, err := noderegtransport.NewServer(noderegtransport.ServerConfig{}, ndHandler)
	if err != nil {
		t.Fatal(err)
	}
	relayHandler := &registryrelay.Handler{
		Store:     store,
		Allowlist: map[string]string{"n1": nodeURI},
		Guard:     guardOK{},
	}
	rlSrv, err := registryrelaytransport.NewServer(registryrelaytransport.ServerConfig{}, relayHandler)
	if err != nil {
		t.Fatal(err)
	}
	combined, err := NewServer(Config{
		BindAddr: "127.0.0.1:0",
		Nodereg:  ndSrv,
		Relay:    rlSrv,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := combined.handler()

	assertNotFound := func(path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("path %q: expected 404 got %d", path, rec.Code)
		}
	}
	assertNotFound("/")
	assertNotFound("/v1/public/api")
	assertNotFound("/v1/internal/elastic-node-registr")
	assertNotFound("/v1/internal/elastic-registry-rela")

	dispatched := func(method, path string) {
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("path %q: expected dispatch to sub-server, got 404", path)
		}
	}
	dispatched(http.MethodPost, noderegtransport.RoutePrefix+"/activate")
	dispatched(http.MethodPost, registryrelaytransport.RoutePrefix+"/get-runtime-node")
}
