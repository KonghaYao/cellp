package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Println(versionLine())
	case "doctor":
		os.Exit(cmdDoctor())
	case "dev":
		os.Exit(cmdDev(os.Args[2:]))
	case "serve":
		os.Exit(cmdServe())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `cellp — private Workers control plane

Usage:
  cellp dev       Start a local platform (no Docker). Deploys cwd if wrangler.jsonc exists.
  cellp serve     Run cellpd from environment (production / Compose).
  cellp doctor    Check celld, offshoot, esbuild, and ports.
  cellp version   Print the build version.

Install:
  curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh

Docs: https://konghayao.github.io/cellp/get-started/
`)
}
