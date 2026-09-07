package serve

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cellp/cellp/internal/api"
	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/autoscaler"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/gc"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/metrics"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func controllerGuardID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

const runShutdownTimeout = 15 * time.Second

// Run starts the cellpd API + gateway until ctx is cancelled.
func Run(ctx context.Context) (retErr error) {
	cfg := config.Load()
	if err := cfg.Ingress.Validate(); err != nil {
		return fmt.Errorf("ingress config: %w", err)
	}
	elasticCfg, err := config.LoadElasticConfig()
	if err != nil {
		return err
	}
	remoteCfg, err := config.LoadRemoteControlConfig(elasticCfg.Enabled)
	if err != nil {
		return err
	}
	if err := config.ValidateServeElastic(elasticCfg, remoteCfg); err != nil {
		return err
	}
	elasticTLS, err := loadElasticTLS(elasticCfg)
	if err != nil {
		return err
	}
	remoteTLS, err := loadRemoteControlTLS(remoteCfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.RegistryDB), 0o755); err != nil {
		return err
	}
	baseStore, err := registry.Open(cfg.RegistryDB)
	if err != nil {
		return err
	}
	defer baseStore.Close()
	log.Printf("registry: SQLite (%s)", cfg.RegistryDB)

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	guardID := controllerGuardID()
	skipGuard := os.Getenv("CELLP_SKIP_CONTROLLER_GUARD") == "1"
	guardAcquired := false
	if !skipGuard && controllerGuardRequired() {
		if err := baseStore.TryAcquireControllerGuard(runCtx, guardID, os.Getpid()); err != nil {
			return fmt.Errorf("singleton controller guard: %w (only one active cellpd writer; set CELLP_SKIP_CONTROLLER_GUARD=1 for tests)", err)
		}
		guardAcquired = true
		log.Printf("controller guard acquired: %s", guardID)
	}
	if err := requireElasticControllerGuard(elasticCfg.Enabled, guardAcquired); err != nil {
		return err
	}

	gw := gateway.New(baseStore)
	activatorCfg, err := activator.ConfigFromEnv()
	if err != nil {
		return err
	}
	activationGuard := scheduler.NewRegistryGuard(baseStore, guardID)
	if err := gw.ConfigureElasticActivator(&activator.RegistryEnsureClient{Store: baseStore, Guard: activationGuard}, activatorCfg); err != nil {
		return fmt.Errorf("elastic activator: %w", err)
	}
	snapshotDone := gw.StartRouteSnapshotPoller(runCtx, 0)
	store := gateway.WrapStore(baseStore, gw)
	lm := gateway.NewListenerManager(gw, baseStore, gw.Config())

	queue := job.NewSQLiteQueue(store)
	bm := branch.New(cfg.OffshootStore, store)
	rm := runtime.New(cfg.CelldBasePort, cfg.S3Endpoint, cfg.S3Region, cfg.CelldBucket, cfg.S3AccessKey, cfg.S3SecretKey)
	rm.SetWorkerEnvLoader(func(ctx context.Context, project, version string) (map[string]string, error) {
		return store.GetVersionEnv(ctx, project, version)
	})
	as := &artifact.Store{
		Bucket:      cfg.ArtifactsBucket,
		LocalDir:    cfg.ArtifactsDir,
		S3Endpoint:  cfg.S3Endpoint,
		S3Region:    cfg.S3Region,
		S3AccessKey: cfg.S3AccessKey,
		S3SecretKey: cfg.S3SecretKey,
	}
	o := orch.New(store, queue, bm, rm, as, cfg)
	o.SetIngressListenerReconciler(lm)
	o.SetRouteSnapshotAck(gw.RouteSnapshotHolder())
	var schedCtrl *scheduler.Controller

	apiSrv := api.NewServer(store, queue, o, rm, cfg)
	apiServer := &http.Server{Addr: cfg.APIAddr(), Handler: apiSrv.Handler()}
	gwServer := &http.Server{Addr: cfg.GatewayAddr(), Handler: gw.Handler()}
	var gwTLSServer *http.Server
	var agentRuntime *elasticAgent
	var remoteRuntime *remoteControlRuntime
	apiDone := make(chan struct{})
	gwDone := make(chan struct{})
	var gwTLSDone chan struct{}
	orchDone := make(chan struct{})
	var autoscalerDone, schedulerDone, metricsDone, gcDone, archiveDone <-chan struct{}
	orchStarted := false
	cleaned := false
	cleanup := func() error {
		if cleaned {
			return nil
		}
		cleaned = true
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), runShutdownTimeout)
		defer shutdownCancel()
		var errs []error
		quiesced := true
		if err := lm.CloseAll(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("ingress listeners: %w", err))
			markQuiescence(err, &quiesced)
		}
		if err := shutdownHTTPServer(shutdownCtx, apiServer, apiDone); err != nil {
			errs = append(errs, fmt.Errorf("api shutdown: %w", err))
			markQuiescence(err, &quiesced)
		}
		if err := shutdownHTTPServer(shutdownCtx, gwServer, gwDone); err != nil {
			errs = append(errs, fmt.Errorf("gateway shutdown: %w", err))
			markQuiescence(err, &quiesced)
		}
		if gwTLSServer != nil {
			if err := shutdownHTTPServer(shutdownCtx, gwTLSServer, gwTLSDone); err != nil {
				errs = append(errs, fmt.Errorf("gateway tls shutdown: %w", err))
				markQuiescence(err, &quiesced)
			}
		}
		if err := gw.ShutdownElasticActivator(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("activator shutdown: %w", err))
			markQuiescence(err, &quiesced)
		}
		runCancel()
		if orchStarted {
			if err := waitBackground(shutdownCtx, orchDone); err != nil {
				errs = append(errs, fmt.Errorf("orchestrator shutdown: %w", err))
				markQuiescence(err, &quiesced)
			}
		}
		for name, done := range map[string]<-chan struct{}{
			"route snapshot poller": snapshotDone,
			"autoscaler":            autoscalerDone,
			"scheduler":             schedulerDone,
			"metrics collector":     metricsDone,
			"gc":                    gcDone,
			"archive reaper":        archiveDone,
		} {
			if done == nil {
				continue
			}
			if err := waitBackground(shutdownCtx, done); err != nil {
				errs = append(errs, fmt.Errorf("%s shutdown: %w", name, err))
				markQuiescence(err, &quiesced)
			}
		}
		if agentRuntime != nil {
			agentQuiesced, err := agentRuntime.quiesce(shutdownCtx)
			if err != nil {
				errs = append(errs, fmt.Errorf("agent quiesce: %w", err))
				markQuiescence(err, &quiesced)
			}
			if !agentQuiesced {
				quiesced = false
			}
		}
		if remoteRuntime != nil {
			if err := remoteRuntime.quiesce(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("remote control quiesce: %w", err))
				markQuiescence(err, &quiesced)
			}
		}
		if err := rm.StopAll(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("runtime shutdown: %w", err))
			markQuiescence(err, &quiesced)
		}
		if quiesced && agentRuntime != nil {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer releaseCancel()
			if err := agentRuntime.releaseLease(releaseCtx); err != nil {
				errs = append(errs, fmt.Errorf("agent lease release: %w", err))
				quiesced = false
			}
		}
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer releaseCancel()
		if err := releaseControllerGuardIfQuiesced(releaseCtx, baseStore, guardID, guardAcquired, quiesced); err != nil {
			errs = append(errs, fmt.Errorf("controller guard release: %w", err))
		}
		return errors.Join(errs...)
	}
	defer func() {
		retErr = errors.Join(retErr, cleanup())
	}()

	reconcileCfg := runtime.LoadReconcileConfig()
	autoscalerDone = autoscaler.Start(runCtx, baseStore, autoscaler.LoadConfig())
	var schedGuard scheduler.ControllerGuardChecker
	var registryGuard scheduler.RegistryGuard
	if guardAcquired {
		registryGuard = scheduler.NewRegistryGuard(baseStore, guardID)
		schedGuard = registryGuard
	}
	if remoteCfg.Enabled {
		if !guardAcquired {
			runCancel()
			return errors.New("remote control requires controller guard")
		}
		remoteRuntime, err = startRemoteControl(runCtx, remoteCfg, remoteTLS, baseStore, registryGuard)
		if err != nil {
			runCancel()
			abortCtx, abortCancel := context.WithTimeout(context.Background(), runShutdownTimeout)
			_ = abortRemoteControlStartup(abortCtx, remoteRuntime)
			abortCancel()
			return err
		}
		log.Printf("controller remote control mTLS listening on %s", remoteRuntime.server.Addr())
	}
	metricsDone = metrics.StartCollector(runCtx, store, rm, reconcileCfg.Interval)
	_ = metrics.Collect(runCtx, store, rm)

	orchStarted = true
	go func() {
		o.Run(runCtx)
		close(orchDone)
	}()
	gcDone = gc.Start(runCtx, store, gc.LoadConfig())
	archiveDone = o.StartArchiveReaper(runCtx, orch.LoadArchiveConfig())

	// Boot ingress reconcile must not use the signal ctx: fleet reconcile / restart races can cancel it before API listens.
	bootIngress, bootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := lm.ReconcileIngressListeners(bootIngress); err != nil {
		bootCancel()
		return fmt.Errorf("ingress listeners boot reconcile: %w", err)
	}
	bootCancel()

	errCh := make(chan error, 4)
	if schedGuard == nil {
		runCancel()
		return errors.New("elastic scheduler requires controller guard")
	}
	if elasticCfg.AgentEmbedded {
		agentRuntime, err = startElasticAgent(runCtx, elasticCfg, elasticTLS, baseStore, rm, errCh)
		if err != nil {
			runCancel()
			return err
		}
	}
	schedCtrl = newElasticSchedulerController(baseStore, schedGuard, elasticCfg, elasticTLS.client)
	o.SetElasticSchedulerTick(func(ctx context.Context) error {
		_, err := schedCtrl.Tick(ctx)
		return err
	})
	schedulerDone = startElasticScheduler(runCtx, schedCtrl, errCh)
	go func() {
		defer close(apiDone)
		log.Printf("cellpd API listening on http://0.0.0.0:%d", cfg.APIPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	go func() {
		defer close(gwDone)
		log.Printf("cellpd Gateway listening on http://0.0.0.0:%d", cfg.GatewayPort)
		if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	if tlsAddr := cfg.GatewayTLSAddr(); tlsAddr != "" && cfg.GatewayTLSCert != "" && cfg.GatewayTLSKey != "" {
		if _, err := os.Stat(cfg.GatewayTLSCert); err != nil {
			log.Printf("gateway TLS disabled: cert not found (%s)", cfg.GatewayTLSCert)
		} else if _, err := os.Stat(cfg.GatewayTLSKey); err != nil {
			log.Printf("gateway TLS disabled: key not found (%s)", cfg.GatewayTLSKey)
		} else {
			gwTLSServer = &http.Server{Addr: tlsAddr, Handler: gw.Handler()}
			gwTLSDone = make(chan struct{})
			go func() {
				defer close(gwTLSDone)
				log.Printf("cellpd Gateway TLS listening on https://0.0.0.0:%d", cfg.GatewayTLSPort)
				if err := gwTLSServer.ListenAndServeTLS(cfg.GatewayTLSCert, cfg.GatewayTLSKey); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()
		}
	}

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		runCancel()
		runErr = err
	}
	return runErr
}
