package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cellp/cellp/internal/version"
)

func versionLine() string {
	return "cellp " + version.Version
}

func binDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func lookTool(name string) string {
	dir := binDir()
	if dir != "" {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		if os.PathSeparator == '\\' {
			p += ".exe"
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func prependPath(dir string) {
	if dir == "" {
		return
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func portFree(hostport string) bool {
	ln, err := net.Listen("tcp", hostport)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func cmdDoctor() int {
	prependPath(binDir())
	ok := true
	check := func(name, hint string, required bool) {
		p := lookTool(name)
		if p == "" {
			if required {
				fmt.Printf("FAIL  %s — not found (%s)\n", name, hint)
				ok = false
			} else {
				fmt.Printf("WARN  %s — not found (%s)\n", name, hint)
			}
			return
		}
		fmt.Printf("ok    %s  %s\n", name, p)
	}
	fmt.Println(versionLine())
	check("celld", "install from GitHub Releases or build celld/", true)
	check("offshoot", "go install github.com/sricola/offshoot/cmd/offshoot@latest", true)
	check("esbuild", "npm i -g esbuild  (needed when the Worker has imports)", false)
	for _, p := range []string{"127.0.0.1:8787", "127.0.0.1:8790", "127.0.0.1:19000"} {
		if portFree(p) {
			fmt.Printf("ok    port %s free\n", p)
		} else {
			fmt.Printf("WARN  port %s in use\n", p)
		}
	}
	if !ok {
		return 1
	}
	return 0
}
