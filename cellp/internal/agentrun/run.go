package agentrun

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay/agentstore"
)

type nodeHeartbeatClient interface {
	Heartbeat(ctx context.Context, req contract.HeartbeatNodeRequest) error
}

type nodeRegistrationClient interface {
	nodeHeartbeatClient
	Activate(ctx context.Context, req contract.ActivateNodeRequest) (contract.ActivateNodeResponse, error)
	Status(ctx context.Context, req contract.StatusNodeRequest) (contract.StatusNodeResponse, error)
	Release(ctx context.Context, req contract.ReleaseNodeRequest) error
}

const shutdownTimeout = 15 * time.Second

var scopeNonceSeq uint64

// Run starts the standalone remote Node Agent until ctx is canceled.
func Run(ctx context.Context, elasticCfg config.ElasticConfig, platformCfg config.Config, deps Deps) error {
	var err error
	var tlsMaterials TLSBundle
	if deps.LoadTLS != nil {
		tlsMaterials, err = deps.LoadTLS(elasticCfg)
	} else {
		tlsMaterials, err = loadTLS(elasticCfg)
	}
	if err != nil {
		return err
	}
	clientTLS := tlsMaterials.Client
	clientTLS.ServerName = elasticCfg.ResolvedControllerTLSServerName()

	ndClient, err := deps.nodeRegClient(elasticCfg, clientTLS)
	if err != nil {
		return fmt.Errorf("nodereg client: %w", err)
	}
	relayClient, err := deps.registryRelayClient(elasticCfg, clientTLS)
	if err != nil {
		return err
	}

	rm, err := deps.manager(platformCfg, elasticCfg)
	if err != nil {
		return err
	}
	var nodeStore agent.NodeStore
	var lifecycleStore agent.LifecycleStore
	if deps.LifecycleStores != nil {
		nodeStore, lifecycleStore = deps.LifecycleStores(relayClient, elasticCfg)
	} else {
		store := agentstore.NewStore(relayClient, elasticCfg.NodeID, elasticCfg.RelayScopeTTL)
		nodeStore, lifecycleStore = store, store
	}
	if relayClient != nil {
		rm.SetWorkerEnvLoader(func(ctx context.Context, project, version string) (map[string]string, error) {
			scope := relayClient.NewRelayScope(elasticCfg.NodeID, elasticCfg.RelayScopeTTL)
			return relayClient.GetVersionEnv(ctx, scope, project, version)
		})
	}
	backend := deps.backend(rm)
	handler := agent.NewLifecycleHandler(true, nodeStore, lifecycleStore, backend)

	server, err := agenttransport.NewServer(agenttransport.ServerConfig{
		Enabled: true, NodeID: elasticCfg.NodeID, BindAddr: elasticCfg.AgentBindAddr,
		NodeIdentityURI: elasticCfg.NodeIdentityURI, AllowedControllerURIs: elasticCfg.AllowedControllerURIs,
		TLS: tlsMaterials.Server, MaxBodyBytes: elasticCfg.MaxBodyBytes, ReplayMaxEntries: elasticCfg.ReplayMaxEntries,
	}, handler)
	if err != nil {
		return err
	}
	server.SetAccepting(false)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	loopsDone := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		defer close(done)
		runErr <- server.Run(runCtx)
	}()

	if err := waitReady(runCtx, server.Ready(), done, runErr); err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		_ = abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, 0)
		return err
	}

	advertiseURL, err := contract.CanonicalAgentBaseURL(elasticCfg.AgentBaseURL)
	if err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		_ = abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, 0)
		return fmt.Errorf("agent advertise url: %w", err)
	}

	now := time.Now().UTC()
	statusScope := registrationScope(elasticCfg.NodeID, 0, contract.MaxControlPlaneScopeTTL)
	status, err := ndClient.Status(runCtx, contract.StatusNodeRequest{Scope: statusScope})
	if err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		_ = abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, 0)
		return fmt.Errorf("nodereg status: %w", err)
	}
	expectedGen, err := expectedActivationGeneration(status, now)
	if err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		_ = abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, 0)
		return err
	}
	actScope := registrationScope(elasticCfg.NodeID, 0, contract.MaxControlPlaneScopeTTL)
	act, err := ndClient.Activate(runCtx, contract.ActivateNodeRequest{
		Scope: actScope, CapacityUnits: elasticCfg.CapacityUnits, AgentBaseURL: advertiseURL,
		IdentityURI: elasticCfg.NodeIdentityURI, Zone: elasticCfg.Zone, ExpectedGeneration: expectedGen,
	})
	if err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		_ = abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, 0)
		return fmt.Errorf("nodereg activate: %w", err)
	}
	generation := act.Generation

	if err := handler.ReconcileNode(runCtx, elasticCfg.NodeID); err != nil {
		ctxErr := runCtx.Err()
		abortCtx, abortCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer abortCancel()
		abortErr := abortStartup(abortCtx, cancel, server, done, nil, ndClient, elasticCfg.NodeID, generation)
		if ctxErr != nil {
			return errors.Join(ctxErr, abortErr)
		}
		return errors.Join(fmt.Errorf("agent boot reconcile: %w", err), abortErr)
	}
	server.SetAccepting(true)

	errCh := make(chan error, 1)
	go func() {
		defer close(loopsDone)
		runHeartbeatReconcileLoops(runCtx, elasticCfg, ndClient, handler, generation, runErr, errCh, cancel)
	}()

	var runErrOut error
	select {
	case <-runCtx.Done():
	case err := <-errCh:
		cancel()
		runErrOut = err
	case err := <-runErr:
		if err != nil && runCtx.Err() == nil {
			cancel()
			runErrOut = fmt.Errorf("agent server: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	quiesced, shutdownErr := quiesce(shutdownCtx, cancel, server, done, loopsDone)
	var releaseErr error
	if generation > 0 && quiesced {
		releaseErr = releaseLease(shutdownCtx, ndClient, elasticCfg.NodeID, generation)
	}
	return errors.Join(runErrOut, shutdownErr, releaseErr)
}

func runHeartbeatReconcileLoops(
	ctx context.Context,
	cfg config.ElasticConfig,
	nd nodeHeartbeatClient,
	handler *agent.Handler,
	generation int64,
	runErr <-chan error,
	errCh chan<- error,
	cancel context.CancelFunc,
) {
	scope := registrationScope(cfg.NodeID, generation, contract.MaxControlPlaneScopeTTL)
	if err := nd.Heartbeat(ctx, contract.HeartbeatNodeRequest{
		Scope: scope, LeaseExpiry: time.Now().UTC().Add(cfg.NodeHeartbeatTTL),
	}); err != nil {
		if ctx.Err() == nil && agent.RuntimeNodeHeartbeatFatal(err) {
			errCh <- fmt.Errorf("agent heartbeat: %w", err)
			cancel()
			return
		}
		if ctx.Err() == nil {
			log.Printf("agent heartbeat transient: %v", err)
		}
	}
	heartbeat := time.NewTicker(cfg.NodeHeartbeatInterval)
	reconcile := time.NewTicker(cfg.AgentReconcileInterval)
	defer heartbeat.Stop()
	defer reconcile.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-runErr:
			if err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("agent server: %w", err)
			}
			cancel()
			return
		case <-heartbeat.C:
			hbScope := registrationScope(cfg.NodeID, generation, contract.MaxControlPlaneScopeTTL)
			if err := nd.Heartbeat(ctx, contract.HeartbeatNodeRequest{
				Scope: hbScope, LeaseExpiry: time.Now().UTC().Add(cfg.NodeHeartbeatTTL),
			}); err != nil {
				if ctx.Err() == nil && agent.RuntimeNodeHeartbeatFatal(err) {
					errCh <- fmt.Errorf("agent heartbeat: %w", err)
					cancel()
					return
				}
				if ctx.Err() == nil {
					log.Printf("agent heartbeat transient: %v", err)
				}
			}
		case <-reconcile.C:
			if err := handler.ReconcileNode(ctx, cfg.NodeID); err != nil {
				if ctx.Err() == nil && agent.ReconcileNodeErrorFatal(err) {
					errCh <- fmt.Errorf("agent reconcile: %w", err)
					cancel()
					return
				}
				if ctx.Err() == nil {
					log.Printf("agent reconcile transient: %v", err)
				}
			}
		}
	}
}

func registrationScope(nodeID string, generation int64, window time.Duration) contract.NodeRegistrationScope {
	now := time.Now().UTC()
	if window <= 0 {
		window = contract.MaxControlPlaneScopeTTL
	}
	if window > contract.MaxControlPlaneScopeTTL {
		window = contract.MaxControlPlaneScopeTTL
	}
	issued := now.Add(-time.Second)
	return contract.NodeRegistrationScope{
		NodeID: nodeID, Generation: generation,
		IssuedAt: issued, ExpiresAt: issued.Add(window),
		Nonce: fmt.Sprintf("agent-%d", atomic.AddUint64(&scopeNonceSeq, 1)),
	}
}

func expectedActivationGeneration(status contract.StatusNodeResponse, now time.Time) (int64, error) {
	if !status.Found {
		return 0, nil
	}
	if status.LeaseExpiry.After(now) {
		return 0, fmt.Errorf("runtime node generation active")
	}
	return status.Generation, nil
}

func waitReady(ctx context.Context, ready <-chan struct{}, done <-chan struct{}, runErr <-chan error) error {
	select {
	case <-ready:
		select {
		case serverErr := <-runErr:
			if serverErr == nil {
				serverErr = errors.New("agent listener stopped during startup")
			}
			return fmt.Errorf("agent listener: %w", serverErr)
		default:
			return nil
		}
	case serverErr := <-runErr:
		if serverErr == nil {
			serverErr = errors.New("agent listener stopped during startup")
		}
		return fmt.Errorf("agent listener: %w", serverErr)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func abortStartup(
	ctx context.Context,
	cancel context.CancelFunc,
	server *agenttransport.Server,
	done, loopsDone <-chan struct{},
	nd nodeRegistrationClient,
	nodeID string,
	generation int64,
) error {
	quiesced, err := quiesce(ctx, cancel, server, done, loopsDone)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if generation > 0 && quiesced && nd != nil {
		if releaseErr := releaseLease(ctx, nd, nodeID, generation); releaseErr != nil {
			errs = append(errs, releaseErr)
		}
	}
	return errors.Join(errs...)
}

func quiesce(ctx context.Context, cancel context.CancelFunc, server *agenttransport.Server, done, loopsDone <-chan struct{}) (bool, error) {
	if cancel != nil {
		cancel()
	}
	quiesced := true
	var errs []error
	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("agent server shutdown: %w", err))
			quiesced = false
		}
	}
	if loopsDone != nil {
		if err := waitChan(ctx, loopsDone); err != nil {
			errs = append(errs, fmt.Errorf("agent loops: %w", err))
			quiesced = false
		}
	}
	if done != nil {
		if err := waitChan(ctx, done); err != nil {
			errs = append(errs, fmt.Errorf("agent server: %w", err))
			quiesced = false
		}
	}
	return quiesced, errors.Join(errs...)
}

func releaseLease(ctx context.Context, nd nodeRegistrationClient, nodeID string, generation int64) error {
	if generation <= 0 {
		return nil
	}
	scope := registrationScope(nodeID, generation, contract.MaxControlPlaneScopeTTL)
	if err := nd.Release(ctx, contract.ReleaseNodeRequest{Scope: scope}); err != nil {
		return fmt.Errorf("agent lease release: %w", err)
	}
	return nil
}

func waitChan(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TLSBundle holds agent listener and outbound controller client materials.
type TLSBundle struct {
	Server agenttransport.TLSMaterials
	Client agenttransport.TLSMaterials
}

func loadTLS(cfg config.ElasticConfig) (TLSBundle, error) {
	server, err := agenttransport.LoadServerTLSMaterials(
		cfg.ServerCertFile, cfg.ServerKeyFile, cfg.ServerCAFile,
		cfg.NodeIdentityURI, cfg.CertificateDenylistFile,
	)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("agent server tls: %w", err)
	}
	client, err := agenttransport.LoadClientTLSMaterials(
		cfg.ClientCertFile, cfg.ClientKeyFile, cfg.ClientCAFile,
		cfg.NodeIdentityURI, cfg.ResolvedControllerTLSServerName(), cfg.CertificateDenylistFile,
	)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("agent client tls: %w", err)
	}
	return TLSBundle{Server: server, Client: client}, nil
}
