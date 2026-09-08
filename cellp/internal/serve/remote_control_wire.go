package serve

import (
	"context"
	"errors"
	"fmt"

	"github.com/cellp/cellp/internal/config"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/controllermtls"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/registry"
)

type controllerGuardAdapter struct {
	inner scheduler.RegistryGuard
}

func (g controllerGuardAdapter) EnsureActiveController(ctx context.Context) error {
	return g.inner.HoldsWriteLock(ctx)
}

func loadRemoteControlTLS(cfg config.RemoteControlConfig) (agenttransport.TLSMaterials, error) {
	if !cfg.Enabled {
		return agenttransport.TLSMaterials{}, nil
	}
	mat, err := agenttransport.LoadServerTLSMaterials(
		cfg.ServerCertFile, cfg.ServerKeyFile, cfg.ServerCAFile,
		cfg.ControllerIdentityURI, cfg.CertificateDenylistFile,
	)
	if err != nil {
		return agenttransport.TLSMaterials{}, fmt.Errorf("remote control server tls: %w", err)
	}
	return mat, nil
}

type remoteControlRuntime struct {
	cancel context.CancelFunc
	server *controllermtls.Server
	done   chan struct{}
}

func (r *remoteControlRuntime) quiesce(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.server != nil {
		if err := r.server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("remote control shutdown: %w", err))
		}
	}
	if r.done != nil {
		if err := waitBackground(ctx, r.done); err != nil {
			errs = append(errs, fmt.Errorf("remote control server: %w", err))
		}
	}
	if r.cancel != nil {
		r.cancel()
	}
	return errors.Join(errs...)
}

func startRemoteControl(
	parent context.Context,
	cfg config.RemoteControlConfig,
	tlsMaterials agenttransport.TLSMaterials,
	store registry.ServingStore,
	guard scheduler.RegistryGuard,
) (*remoteControlRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := guard.HoldsWriteLock(parent); err != nil {
		return nil, fmt.Errorf("remote control requires active controller guard: %w", err)
	}
	guardAdapter := controllerGuardAdapter{inner: guard}
	shared := noderegtransport.ServerConfig{
		BindAddr:              cfg.BindAddr,
		ControllerIdentityURI: cfg.ControllerIdentityURI,
		AllowedNodeURIs:       cfg.AllowedNodeURIs(),
		TLS:                   tlsMaterials,
		MaxBodyBytes:          cfg.MaxBodyBytes,
		ReplayMaxEntries:      cfg.ReplayMaxEntries,
	}
	noderegHandler := &nodereg.Handler{
		Store:        store,
		Allowlist:    cfg.NodeAllowlist,
		Guard:        guardAdapter,
		HeartbeatTTL: cfg.NodeHeartbeatTTL,
	}
	noderegSrv, err := noderegtransport.NewServer(shared, noderegHandler)
	if err != nil {
		return nil, err
	}
	relayHandler := &registryrelay.Handler{
		Store:     store,
		Allowlist: cfg.NodeIdentityAllowlist(),
		Guard:     guardAdapter,
	}
	relaySrv, err := registryrelaytransport.NewServer(registryrelaytransport.ServerConfig{
		BindAddr:              cfg.BindAddr,
		ControllerIdentityURI: cfg.ControllerIdentityURI,
		AllowedNodeURIs:       cfg.AllowedNodeURIs(),
		TLS:                   tlsMaterials,
		MaxBodyBytes:          cfg.MaxBodyBytes,
		ReplayMaxEntries:      cfg.ReplayMaxEntries,
	}, relayHandler)
	if err != nil {
		return nil, err
	}
	combined, err := controllermtls.NewServer(controllermtls.Config{
		BindAddr:              cfg.BindAddr,
		ControllerIdentityURI: cfg.ControllerIdentityURI,
		TLS:                   tlsMaterials,
		Nodereg:               noderegSrv,
		Relay:                 relaySrv,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		defer close(done)
		runErr <- combined.Run(ctx)
	}()
	if err := controllermtls.WaitForStartup(parent, combined.Ready, done, runErr); err != nil {
		cancel()
		<-done
		return nil, err
	}
	return &remoteControlRuntime{cancel: cancel, server: combined, done: done}, nil
}

func abortRemoteControlStartup(ctx context.Context, rt *remoteControlRuntime) error {
	if rt == nil {
		return nil
	}
	return rt.quiesce(ctx)
}
