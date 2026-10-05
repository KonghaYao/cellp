// genelasticdevcerts writes dev mTLS material for embedded Node Agent (AD-15 local stack).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cellp/cellp/internal/elasticdevcerts"
)

func main() {
	out := flag.String("dir", "", "output directory for PEM files")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: genelasticdevcerts --dir <path>")
		os.Exit(2)
	}
	if err := elasticdevcerts.Generate(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(*out)
}
