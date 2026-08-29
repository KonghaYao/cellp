package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/cellp/cellp/internal/gc"
	"github.com/cellp/cellp/internal/registry"
)

func main() {
	dbPath := os.Getenv("CELLP_REGISTRY_DB")
	if dbPath == "" {
		dbPath = "./dev/data/cellp-registry.sqlite"
	}
	store, err := registry.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	cfg := gc.LoadConfig()
	res, err := gc.RunOnce(context.Background(), store, cfg.Retention)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gc: purged jobs=%d versions=%d (retention=%v)\n", res.JobsDeleted, res.VersionsDeleted, cfg.Retention)
}
