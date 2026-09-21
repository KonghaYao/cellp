package serve

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

type elasticTLS struct {
	server transport.TLSMaterials
	client transport.TLSMaterials
}

func loadElasticTLS(cfg config.ElasticConfig) (elasticTLS, error) {
	if !cfg.Enabled {
		return elasticTLS{}, nil
	}
	serverName := strings.TrimSpace(cfg.TLSServerName)
	client, err := transport.LoadClientTLSMaterials(cfg.ClientCertFile, cfg.ClientKeyFile, cfg.ClientCAFile, cfg.ControllerIdentityURI, serverName, cfg.CertificateDenylistFile)
	if err != nil {
		return elasticTLS{}, fmt.Errorf("agent client tls: %w", err)
	}
	if !cfg.AgentEmbedded {
		return elasticTLS{client: client}, nil
	}
	server, err := transport.LoadServerTLSMaterials(cfg.ServerCertFile, cfg.ServerKeyFile, cfg.ServerCAFile, cfg.NodeIdentityURI, cfg.CertificateDenylistFile)
	if err != nil {
		return elasticTLS{}, fmt.Errorf("agent server tls: %w", err)
	}
	return elasticTLS{server: server, client: client}, nil
}

// NewRuntimeNodeClient is the production controller-side Agent factory.
func NewRuntimeNodeClient(node contract.RuntimeNode, cfg config.ElasticConfig, materials transport.TLSMaterials) (*transport.Client, error) {
	tlsMat := materials
	if host := agentBaseURLHostname(node.AgentBaseURL); host != "" {
		tlsMat.ServerName = host
	}
	return transport.NewClient(transport.ClientConfig{
		BaseURL: node.AgentBaseURL, ExpectedNodeURI: node.IdentityURI,
		ControllerIdentityURI: cfg.ControllerIdentityURI, TLS: tlsMat, MaxBodyBytes: cfg.MaxBodyBytes,
	})
}

func agentBaseURLHostname(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

func schedulerClientFactory(cfg config.ElasticConfig, materials transport.TLSMaterials) scheduler.ClientFactory {
	return func(node contract.RuntimeNode) (scheduler.RuntimeNodeClient, error) {
		return NewRuntimeNodeClient(node, cfg, materials)
	}
}

func newElasticSchedulerController(store registry.ServingStore, guard scheduler.ControllerGuardChecker, cfg config.ElasticConfig, materials transport.TLSMaterials) *scheduler.Controller {
	return &scheduler.Controller{
		Store:   scheduler.RegistryStoreFromServing(store),
		Guard:   guard,
		Clients: schedulerClientFactory(cfg, materials),
	}
}

func startElasticScheduler(parent context.Context, ctrl *scheduler.Controller, errCh chan<- error) <-chan struct{} {
	return scheduler.Start(parent, ctrl, scheduler.LoadConfig(), errCh)
}

type elasticAgent struct {
	cancel     context.CancelFunc
	server     *transport.Server
	done       chan struct{}
	loopsDone  chan struct{}
	store      registry.ServingStore
	nodeID     string
	generation int64
}

func abortElasticStartup(ctx context.Context, cancel context.CancelFunc, store registry.ServingStore, nodeID string, generation int64, server *transport.Server, done, loopsDone <-chan struct{}) error {
	quiesced, err := quiesceElasticParts(ctx, cancel, server, done, loopsDone)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if generation > 0 && quiesced {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer releaseCancel()
		if releaseErr := releaseRuntimeNodeLeaseCAS(releaseCtx, store, nodeID, generation); releaseErr != nil {
			errs = append(errs, fmt.Errorf("agent lease release: %w", releaseErr))
		}
	}
	return errors.Join(errs...)
}

func quiesceElasticParts(ctx context.Context, cancel context.CancelFunc, server *transport.Server, done, loopsDone <-chan struct{}) (bool, error) {
	if cancel != nil {
		cancel()
	}
	var errs []error
	quiesced := true
	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("agent server shutdown: %w", err))
			markQuiescence(err, &quiesced)
		}
	}
	if loopsDone != nil {
		if err := waitBackground(ctx, loopsDone); err != nil {
			errs = append(errs, fmt.Errorf("agent loops: %w", err))
			markQuiescence(err, &quiesced)
		}
	}
	if done != nil {
		if err := waitBackground(ctx, done); err != nil {
			errs = append(errs, fmt.Errorf("agent server: %w", err))
			markQuiescence(err, &quiesced)
		}
	}
	return quiesced, errors.Join(errs...)
}

func startElasticAgent(parent context.Context, cfg config.ElasticConfig, tlsMaterials elasticTLS, store registry.ServingStore, manager *runtime.Manager, errCh chan<- error) (*elasticAgent, error) {
	existing, err := store.GetRuntimeNode(parent, cfg.NodeID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	generation, err := nextRuntimeNodeGeneration(existing, now)
	if err != nil {
		return nil, err
	}
	backend := agent.ManagerBackend{Manager: manager}
	handler := agent.NewLifecycleFromRegistry(true, store, backend)
	server, err := transport.NewServer(transport.ServerConfig{
		Enabled: true, NodeID: cfg.NodeID, BindAddr: cfg.AgentBindAddr, NodeIdentityURI: cfg.NodeIdentityURI,
		AllowedControllerURIs: cfg.AllowedControllerURIs, TLS: tlsMaterials.server,
		MaxBodyBytes: cfg.MaxBodyBytes, ReplayMaxEntries: cfg.ReplayMaxEntries,
	}, handler)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	loopsDone := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		defer close(done)
		runErr <- server.Run(ctx)
	}()
	select {
	case <-server.Ready():
		select {
		case serverErr := <-runErr:
			cancel()
			<-done
			if serverErr == nil {
				serverErr = errors.New("agent listener stopped during startup")
			}
			return nil, fmt.Errorf("agent listener: %w", serverErr)
		default:
		}
	case err := <-runErr:
		cancel()
		<-done
		return nil, fmt.Errorf("agent listener: %w", err)
	case <-parent.Done():
		cancel()
		<-done
		return nil, parent.Err()
	}
	node := contract.RuntimeNode{
		NodeID: cfg.NodeID, CapacityUnits: cfg.CapacityUnits, Generation: generation,
		LeaseExpiry: now.Add(cfg.NodeHeartbeatTTL), AgentBaseURL: cfg.AgentBaseURL,
		IdentityURI: cfg.NodeIdentityURI, Zone: cfg.Zone,
	}
	if err := store.ActivateRuntimeNode(parent, node, generation-1); err != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), runShutdownTimeout)
		abortErr := abortElasticStartup(abortCtx, cancel, store, cfg.NodeID, 0, server, done, nil)
		abortCancel()
		return nil, errors.Join(err, abortErr)
	}
	bootResult := make(chan error, 1)
	go func() {
		defer close(loopsDone)
		runElasticAgentLoops(ctx, cancel, cfg, func(ctx context.Context) error {
			return store.RenewRuntimeNodeLease(ctx, cfg.NodeID, generation, time.Now().UTC().Add(cfg.NodeHeartbeatTTL))
		}, func(ctx context.Context) error {
			return handler.ReconcileNode(ctx, cfg.NodeID)
		}, runErr, bootResult, errCh)
	}()
	var bootErr error
	select {
	case bootErr = <-bootResult:
	case <-ctx.Done():
		bootErr = ctx.Err()
		select {
		case err := <-bootResult:
			if err != nil {
				bootErr = err
			}
		default:
		}
	}
	if bootErr == nil {
		bootErr = ctx.Err()
	}
	if bootErr != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), runShutdownTimeout)
		abortErr := abortElasticStartup(abortCtx, cancel, store, cfg.NodeID, generation, server, done, loopsDone)
		abortCancel()
		return nil, errors.Join(fmt.Errorf("agent boot reconcile: %w", bootErr), abortErr)
	}
	return &elasticAgent{
		cancel: cancel, server: server, done: done, loopsDone: loopsDone,
		store: store, nodeID: cfg.NodeID, generation: generation,
	}, nil
}

// runElasticAgentLoops 将续租与串行的 boot/周期 reconcile 分离；返回前取消并等待 worker。
func runElasticAgentLoops(ctx context.Context, cancel context.CancelFunc, cfg config.ElasticConfig, renew, reconcile func(context.Context) error, runErr <-chan error, bootResult chan<- error, errCh chan<- error) {
	defer cancel()
	fatal := func(kind string, err error) {
		// A fatal takes the node offline and cancels the process context, so the first
		// cause must be readable in the log even when errCh is already full: the
		// shutdown messages that follow are consequences, not the reason.
		log.Printf("agent %s fatal: %v", kind, err)
		select {
		case errCh <- fmt.Errorf("agent %s: %w", kind, err):
		default:
			// 已有 fatal 等待处理时也不能阻塞关闭。
		}
		cancel()
	}
	if err := renew(ctx); err != nil && ctx.Err() == nil {
		if agent.RuntimeNodeHeartbeatFatal(err) {
			fatal("heartbeat", err)
			return
		}
		log.Printf("agent heartbeat transient: %v", err)
	}
	workerDone := make(chan struct{})
	reconcileFatal := make(chan error, 1)
	go func() {
		defer close(workerDone)
		err := reconcile(ctx)
		bootResult <- err
		if err != nil {
			cancel()
			return
		}
		ticker := time.NewTicker(cfg.AgentReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if err := reconcile(ctx); err != nil && ctx.Err() == nil {
					if agent.ReconcileNodeErrorFatal(err) {
						reconcileFatal <- err
						return
					}
					log.Printf("agent reconcile transient: %v", err)
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-workerDone
	}()
	heartbeat := time.NewTicker(cfg.NodeHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-runErr:
			if err != nil && ctx.Err() == nil {
				fatal("server", err)
			}
			return
		case err := <-reconcileFatal:
			fatal("reconcile", err)
			return
		case <-heartbeat.C:
			if err := renew(ctx); err != nil && ctx.Err() == nil {
				if agent.RuntimeNodeHeartbeatFatal(err) {
					fatal("heartbeat", err)
					return
				}
				log.Printf("agent heartbeat transient: %v", err)
			}
		}
	}
}

func (a *elasticAgent) quiesce(ctx context.Context) (bool, error) {
	if a == nil {
		return true, nil
	}
	return quiesceElasticParts(ctx, a.cancel, a.server, a.done, a.loopsDone)
}

func (a *elasticAgent) releaseLease(ctx context.Context) error {
	if a == nil || a.generation <= 0 {
		return nil
	}
	return releaseRuntimeNodeLeaseCAS(ctx, a.store, a.nodeID, a.generation)
}

func (a *elasticAgent) shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	quiesced, err := a.quiesce(ctx)
	if !quiesced {
		return err
	}
	if releaseErr := a.releaseLease(ctx); releaseErr != nil {
		err = errors.Join(err, fmt.Errorf("agent lease release: %w", releaseErr))
	}
	return err
}
