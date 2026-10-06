package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds Dashboard static + same-origin proxy settings.
type Config struct {
	StaticDir   string
	APIPort     int
	GatewayPort int
}

// NewHandler serves the Dashboard SPA and proxies /v1, /metrics, and /__gateway
// to cellpd API and Gateway on localhost (same behavior as former nginx sidecar).
func NewHandler(cfg Config) (http.Handler, error) {
	root := strings.TrimSpace(cfg.StaticDir)
	if root == "" {
		return nil, fmt.Errorf("dashboard: static dir is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("dashboard: static dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dashboard: static dir %q is not a directory", root)
	}

	apiTarget, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cfg.APIPort))
	if err != nil {
		return nil, err
	}
	gwTarget, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cfg.GatewayPort))
	if err != nil {
		return nil, err
	}

	apiProxy := httputil.NewSingleHostReverseProxy(apiTarget)
	gwProxy := httputil.NewSingleHostReverseProxy(gwTarget)

	fileServer := http.FileServer(http.Dir(root))
	indexPath := filepath.Join(root, "index.html")

	mux := http.NewServeMux()
	mux.Handle("/v1/", apiProxy)
	mux.Handle("/metrics", apiProxy)
	mux.HandleFunc("/__gateway", gatewayProxy(gwProxy))
	mux.HandleFunc("/__gateway/", gatewayProxy(gwProxy))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if path := filepath.Join(root, filepath.Clean("/"+r.URL.Path)); strings.HasPrefix(path, root) {
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}
		http.ServeFile(w, r, indexPath)
	})

	return mux, nil
}

func gatewayProxy(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := strings.TrimSpace(r.URL.Query().Get("__cellp_host"))
		if host == "" {
			host = r.Host
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/__gateway")
		if r2.URL.Path == "" {
			r2.URL.Path = "/"
		}
		r2.Host = host
		r2.Header.Set("Host", host)
		proxy.ServeHTTP(w, r2)
	}
}
