package serve

import (
	"context"
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
	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/gc"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/metrics"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

// Run starts the cellpd API + gateway until ctx is cancelled.
func Run(ctx context.Context) error {
	cfg := config.Load()
	if err := cfg.Ingress.Validate(); err != nil {
		return fmt.Errorf("ingress config: %w", err)
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

	gw := gateway.New(baseStore)
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

	reconcileCfg := runtime.LoadReconcileConfig()
	if started, skipped, err := runtime.ReconcileFleet(ctx, store, rm); err != nil {
		log.Printf("reconcile: boot fleet reconcile failed: %v", err)
	} else if started > 0 || skipped > 0 {
		log.Printf("reconcile: boot started=%d skipped=%d", started, skipped)
	}
	runtime.StartReconciler(ctx, store, rm, reconcileCfg)
	metrics.StartCollector(ctx, store, rm, reconcileCfg.Interval)
	_ = metrics.Collect(ctx, store, rm)

	go o.Run(ctx)
	gc.Start(ctx, store, gc.LoadConfig())
	o.StartArchiveReaper(ctx, orch.LoadArchiveConfig())

	apiSrv := api.NewServer(store, queue, o, rm, cfg)

	// Boot ingress reconcile must not use the signal ctx: fleet reconcile / restart races can cancel it before API listens.
	bootIngress, bootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := lm.ReconcileIngressListeners(bootIngress); err != nil {
		bootCancel()
		return fmt.Errorf("ingress listeners boot reconcile: %w", err)
	}
	bootCancel()

	apiServer := &http.Server{Addr: cfg.APIAddr(), Handler: apiSrv.Handler()}
	gwServer := &http.Server{Addr: cfg.GatewayAddr(), Handler: gw.Handler()}

	errCh := make(chan error, 3)
	go func() {
		log.Printf("cellpd API listening on http://0.0.0.0:%d", cfg.APIPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	go func() {
		log.Printf("cellpd Gateway listening on http://0.0.0.0:%d", cfg.GatewayPort)
		if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	var gwTLSServer *http.Server
	if tlsAddr := cfg.GatewayTLSAddr(); tlsAddr != "" && cfg.GatewayTLSCert != "" && cfg.GatewayTLSKey != "" {
		if _, err := os.Stat(cfg.GatewayTLSCert); err != nil {
			log.Printf("gateway TLS disabled: cert not found (%s)", cfg.GatewayTLSCert)
		} else if _, err := os.Stat(cfg.GatewayTLSKey); err != nil {
			log.Printf("gateway TLS disabled: key not found (%s)", cfg.GatewayTLSKey)
		} else {
			gwTLSServer = &http.Server{Addr: tlsAddr, Handler: gw.Handler()}
			go func() {
				log.Printf("cellpd Gateway TLS listening on https://0.0.0.0:%d", cfg.GatewayTLSPort)
				if err := gwTLSServer.ListenAndServeTLS(cfg.GatewayTLSCert, cfg.GatewayTLSKey); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()
		}
	}

	shutdownAll := func() {
		lm.CloseAll(context.Background())
		_ = apiServer.Shutdown(context.Background())
		_ = gwServer.Shutdown(context.Background())
		if gwTLSServer != nil {
			_ = gwTLSServer.Shutdown(context.Background())
		}
		if err := rm.StopAll(context.Background()); err != nil {
			log.Printf("runtime shutdown: %v", err)
		}
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		shutdownAll()
		return err
	}
	shutdownAll()
	return nil
}
