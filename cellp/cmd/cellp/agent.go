package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cellp/cellp/internal/agentrun"
	"github.com/cellp/cellp/internal/config"
)

// standaloneAgentRun is injectable in tests (cmdAgent exit semantics).
var standaloneAgentRun = agentrun.Run

func cmdAgent(args []string) int {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Print(`cellp agent — standalone remote Node Agent

Requires full agent TLS/controller env (elastic runtime is required; do not set CELLP_ELASTIC_RUNTIME=off).
Does not start cellpd or open the controller registry database.

  -h, --help   show this help
`)
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unknown argument %q\n\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: cellp agent")
		return 2
	}

	cfg, err := config.LoadStandaloneAgentConfig()
	if err != nil {
		log.Println(err)
		return 1
	}
	platformCfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := standaloneAgentRun(ctx, cfg, platformCfg, agentrun.Deps{}); err != nil {
		return finishAgentRun(ctx, err)
	}
	return 0
}

func finishAgentRun(ctx context.Context, err error) int {
	exitCode, logLine := agentrun.RunExitOutcome(ctx, err)
	if exitCode == 0 {
		return 0
	}
	if logLine != "" {
		log.Println(logLine)
	}
	return exitCode
}
