package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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

func main() {
	cfg := config.Load()

	if err := os.MkdirAll(filepath.Dir(cfg.RegistryDB), 0o755); err != nil {
		log.Fatalf("mkdir registry: %v", err)
	}
	baseStore, err := registry.Open(cfg.RegistryDB)
	if err != nil {
		log.Fatal(err)
	}
	defer baseStore.Close()
	log.Printf("registry: SQLite (%s)", cfg.RegistryDB)

	gw := gateway.New(baseStore)
	store := gateway.WrapStore(baseStore, gw)

	queue := job.NewSQLiteQueue(store)
	bm := branch.New(cfg.OffshootStore, store)
	rm := runtime.New(cfg.CelldBasePort, cfg.S3Endpoint, cfg.S3Region, cfg.CelldBucket, cfg.S3AccessKey, cfg.S3SecretKey)
	as := &artifact.Store{
		Bucket:      cfg.ArtifactsBucket,
		LocalDir:    cfg.ArtifactsDir,
		S3Endpoint:  cfg.S3Endpoint,
		S3Region:    cfg.S3Region,
		S3AccessKey: cfg.S3AccessKey,
		S3SecretKey: cfg.S3SecretKey,
	}
	o := orch.New(store, queue, bm, rm, as, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	apiSrv := api.NewServer(store, queue, o, rm, cfg)

	apiServer := &http.Server{Addr: cfg.APIAddr(), Handler: apiSrv.Handler()}
	gwServer := &http.Server{Addr: cfg.GatewayAddr(), Handler: gw.Handler()}

	go func() {
		log.Printf("cellpd API listening on http://0.0.0.0:%d", cfg.APIPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	go func() {
		log.Printf("cellpd Gateway listening on http://0.0.0.0:%d", cfg.GatewayPort)
		if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
	cancel()
	_ = apiServer.Shutdown(context.Background())
	_ = gwServer.Shutdown(context.Background())
}
