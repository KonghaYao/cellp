package serve

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	apiServer := &http.Server{Addr: cfg.APIAddr(), Handler: apiSrv.Handler()}
	gwServer := &http.Server{Addr: cfg.GatewayAddr(), Handler: gw.Handler()}

	errCh := make(chan error, 2)
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

	select {
	case <-ctx.Done():
	case err := <-errCh:
		_ = apiServer.Shutdown(context.Background())
		_ = gwServer.Shutdown(context.Background())
		return err
	}
	_ = apiServer.Shutdown(context.Background())
	_ = gwServer.Shutdown(context.Background())
	return nil
}
