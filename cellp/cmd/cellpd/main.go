package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cellp/cellp/internal/serve"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := serve.Run(ctx); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
