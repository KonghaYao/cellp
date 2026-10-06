package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDashboardStaticAndAPIProxy(t *testing.T) {
	t.Parallel()

	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>dashboard</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "dashboard.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(apiSrv.Close)

	gwSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/probe" {
			_, _ = w.Write([]byte(r.Host))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(gwSrv.Close)

	apiURL := strings.TrimPrefix(apiSrv.URL, "http://")
	gwURL := strings.TrimPrefix(gwSrv.URL, "http://")
	apiPort := strings.Split(apiURL, ":")[1]
	gwPort := strings.Split(gwURL, ":")[1]

	handler, err := NewHandler(Config{
		StaticDir:   staticDir,
		APIPort:     mustPort(t, apiPort),
		GatewayPort: mustPort(t, gwPort),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	t.Run("spa index", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/projects/demo-app")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "dashboard") {
			t.Fatalf("got %d body=%q", res.StatusCode, body)
		}
	})

	t.Run("static asset", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/dashboard.js")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "console.log") {
			t.Fatalf("got %d body=%q", res.StatusCode, body)
		}
	})

	t.Run("api proxy", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
			t.Fatalf("got %d body=%q", res.StatusCode, body)
		}
	})

	t.Run("gateway proxy host override", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/__gateway/probe?__cellp_host=preview.example.test")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || string(body) != "preview.example.test" {
			t.Fatalf("got %d body=%q", res.StatusCode, body)
		}
	})
}

func mustPort(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
