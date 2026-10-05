package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

func portListener(hostport string) string {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return ""
	}
	if host == "" {
		host = "127.0.0.1"
	}
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, host+":"+port) || strings.Contains(line, "*:"+port) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[0] + " (pid " + fields[1] + ")"
			}
		}
	}
	return "unknown process"
}

func cellpdHealthURL() string {
	base := strings.TrimSpace(os.Getenv("PLATFORM_URL"))
	if base == "" {
		port := strings.TrimSpace(os.Getenv("PLATFORM_PORT"))
		if port == "" {
			port = "8790"
		}
		base = "http://127.0.0.1:" + port
	}
	return strings.TrimRight(base, "/") + "/v1/health"
}

func probeHTTP(url string) (int, error) {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, nil
}

func checkPort(hostport string, cellpdOK bool) {
	if portFree(hostport) {
		fmt.Printf("ok    port %s free\n", hostport)
		return
	}
	if cellpdOK && strings.HasSuffix(hostport, ":8790") {
		fmt.Printf("ok    port %s in use (cellpd)\n", hostport)
		return
	}
	if who := portListener(hostport); who != "" {
		fmt.Printf("WARN  port %s in use — %s\n", hostport, who)
		return
	}
	fmt.Printf("WARN  port %s in use\n", hostport)
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
	check("celld", "mise use github:KonghaYao/cellp@latest or scripts/install.sh", true)
	check("offshoot", "bundled in cellp release tarball", true)
	check("esbuild", "bundled in cellp release tarball (Worker imports)", false)

	cellpdCode, cellpdErr := probeHTTP(cellpdHealthURL())
	cellpdOK := cellpdErr == nil && cellpdCode > 0 && cellpdCode < 500
	if cellpdOK {
		fmt.Printf("ok    cellpd  %s (HTTP %d)\n", cellpdHealthURL(), cellpdCode)
	} else if cellpdErr != nil {
		fmt.Printf("WARN  cellpd not reachable at %s (%v)\n", cellpdHealthURL(), cellpdErr)
	} else {
		fmt.Printf("WARN  cellpd unhealthy at %s (HTTP %d)\n", cellpdHealthURL(), cellpdCode)
	}

	gwURL := strings.TrimSpace(os.Getenv("GATEWAY_URL"))
	if gwURL == "" {
		port := strings.TrimSpace(os.Getenv("GATEWAY_PORT"))
		if port == "" {
			port = "8787"
		}
		gwURL = "http://127.0.0.1:" + port + "/health"
	} else if !strings.HasSuffix(gwURL, "/health") {
		gwURL = strings.TrimRight(gwURL, "/") + "/health"
	}
	gwCode, gwErr := probeHTTP(gwURL)
	if gwErr == nil && gwCode == 200 {
		fmt.Printf("ok    gateway  %s\n", gwURL)
	} else if gwErr != nil {
		fmt.Printf("WARN  gateway not reachable at %s (%v)\n", gwURL, gwErr)
	} else {
		fmt.Printf("WARN  gateway unhealthy at %s (HTTP %d)\n", gwURL, gwCode)
	}

	s3Addr := envOr("CELLP_S3_ADDR", "127.0.0.1:19000")
	for _, p := range []string{"127.0.0.1:8787", "127.0.0.1:8790", s3Addr, "127.0.0.1:19443"} {
		checkPort(p, cellpdOK)
	}
	store := strings.TrimSpace(os.Getenv("CELLP_STORE"))
	if store == "" {
		store = "local (embedded S3)"
	}
	fmt.Printf("ok    store mode  %s\n", store)

	if !ok {
		return 1
	}
	return 0
}
