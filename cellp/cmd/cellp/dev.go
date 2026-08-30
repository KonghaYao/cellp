package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cellp/cellp/internal/locals3"
	"github.com/cellp/cellp/internal/serve"
)

func cmdServe() int {
	prependPath(binDir())
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := serve.Run(ctx); err != nil {
		log.Println(err)
		return 1
	}
	return 0
}

func cmdDev(args []string) int {
	home := strings.TrimSpace(os.Getenv("CELLP_HOME"))
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			log.Println(err)
			return 1
		}
		home = filepath.Join(h, ".cellp")
	}
	skipDeploy := false
	project := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--home":
			i++
			if i >= len(args) {
				log.Println("--home needs a directory")
				return 2
			}
			home = args[i]
		case "--no-deploy":
			skipDeploy = true
		case "--project":
			i++
			if i >= len(args) {
				log.Println("--project needs an id")
				return 2
			}
			project = args[i]
		case "-h", "--help":
			fmt.Print(`cellp dev — local platform without Docker

  --home DIR       data directory (default ~/.cellp)
  --project ID     project id when deploying cwd (default: wrangler name)
  --no-deploy      do not upload cwd Worker
`)
			return 0
		}
	}

	data := filepath.Join(home, "data")
	for _, d := range []string{
		filepath.Join(data, "artifacts"),
		filepath.Join(data, "offshoot-store"),
		filepath.Join(data, "offshoot-checkouts"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Println(err)
			return 1
		}
	}

	prependPath(binDir())
	if lookTool("celld") == "" {
		log.Println("celld not found. Run: cellp doctor")
		log.Println("Install: curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh")
		return 1
	}

	s3addr := envOr("CELLP_S3_ADDR", "127.0.0.1:19000")
	apiPort := envOr("PLATFORM_PORT", "8790")
	gwPort := envOr("GATEWAY_PORT", "8787")
	for _, p := range []string{"127.0.0.1:" + gwPort, "127.0.0.1:" + apiPort, s3addr} {
		if !portFree(p) {
			log.Printf("port %s is in use — stop the other process, or set GATEWAY_PORT / PLATFORM_PORT / CELLP_S3_ADDR", p)
			return 1
		}
	}

	s3, err := locals3.Start(s3addr, filepath.Join(data, "s3.bolt"))
	if err != nil {
		log.Printf("local s3: %v", err)
		return 1
	}
	defer s3.Close()
	log.Printf("local s3 (no Docker) %s", s3.Addr)

	applyDevEnv(data, s3.Addr, apiPort, gwPort)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- serve.Run(ctx)
	}()

	if err := waitHTTP("http://127.0.0.1:"+apiPort+"/v1/health", 20*time.Second); err != nil {
		log.Printf("api did not become ready: %v", err)
		return 1
	}
	select {
	case err := <-serveErr:
		if err != nil {
			log.Printf("cellpd: %v", err)
			return 1
		}
	default:
	}

	cwd, _ := os.Getwd()
	if !skipDeploy {
		if _, err := os.Stat(filepath.Join(cwd, "wrangler.jsonc")); err == nil {
			if err := deployCwd(cwd, data, project); err != nil {
				log.Printf("deploy cwd: %v", err)
			}
		} else {
			log.Printf("no wrangler.jsonc in %s — platform is up. See https://konghayao.github.io/cellp/build/", cwd)
		}
	}

	fmt.Printf("\n  API      http://127.0.0.1:%s\n", apiPort)
	fmt.Printf("  Gateway  http://127.0.0.1:%s\n", gwPort)
	fmt.Printf("  Data     %s\n", data)
	fmt.Printf("  Stop     Ctrl-C\n\n")

	select {
	case <-ctx.Done():
		return 0
	case err := <-serveErr:
		if err != nil {
			log.Printf("cellpd: %v", err)
			return 1
		}
		return 0
	}
}

func applyDevEnv(data, s3URL, apiPort, gwPort string) {
	setDefault := func(k, v string) {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	setDefault("CELLP_REGISTRY_DB", filepath.Join(data, "cellp-registry.sqlite"))
	setDefault("ARTIFACTS_DIR", filepath.Join(data, "artifacts"))
	setDefault("OFFSHOOT_STORE", filepath.Join(data, "offshoot-store"))
	setDefault("OFFSHOOT_CHECKOUTS", filepath.Join(data, "offshoot-checkouts"))
	setDefault("S3_ENDPOINT", s3URL)
	setDefault("AWS_ACCESS_KEY_ID", "cellpdev")
	setDefault("AWS_SECRET_ACCESS_KEY", "cellpdev")
	setDefault("AWS_REGION", "us-east-1")
	setDefault("CELLP_DEPLOY_TOKEN", "dev-local-token")
	setDefault("CELLP_ADMIN_TOKEN", "dev-local-token")
	setDefault("PLATFORM_TOKEN", "dev-local-token")
	setDefault("GATEWAY_URL", "http://127.0.0.1:"+gwPort)
	setDefault("PLATFORM_URL", "http://127.0.0.1:"+apiPort)
	setDefault("CELLP_ARTIFACTS_BUCKET", "cellp-artifacts")
	setDefault("CELLD_BUCKET", "s3://cellp-celld/demo-app")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func waitHTTP(url string, d time.Duration) error {
	deadline := time.Now().Add(d)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func deployCwd(cwd, data, project string) error {
	if project == "" {
		project = wranglerName(cwd)
	}
	if project == "" {
		project = "app"
	}
	token := envOr("CELLP_ADMIN_TOKEN", envOr("CELLP_DEPLOY_TOKEN", "dev-local-token"))
	deployTok := envOr("CELLP_DEPLOY_TOKEN", "dev-local-token")
	api := strings.TrimRight(envOr("PLATFORM_URL", "http://127.0.0.1:8790"), "/")
	version := envOr("CELLP_DEV_VERSION", "dev")
	parent := ""
	if st, body, err := apiJSON(http.MethodGet, api+"/v1/projects/"+project, token, nil); err == nil && st == http.StatusOK {
		var proj struct {
			ProdVersionID *string `json:"prod_version_id"`
		}
		_ = json.Unmarshal(body, &proj)
		if proj.ProdVersionID != nil {
			parent = *proj.ProdVersionID
		}
	}
	if st, _, err := apiJSON(http.MethodGet, api+"/v1/projects/"+project+"/versions/"+version, token, nil); err == nil && st == http.StatusOK {
		version = fmt.Sprintf("%s-%d", version, time.Now().Unix())
	}
	dest := filepath.Join(data, "artifacts", project, version)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".") {
			continue
		}
		if err := copyPath(filepath.Join(cwd, name), filepath.Join(dest, name)); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	body := fmt.Sprintf(`{"id":%q}`, version)
	if parent != "" && parent != version {
		body = fmt.Sprintf(`{"id":%q,"parent_version_id":%q}`, version, parent)
	}
	st, respBody, err := apiJSON(http.MethodPost, api+"/v1/projects/"+project+"/versions", deployTok, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST /versions: %w", err)
	}
	if st >= 300 {
		return fmt.Errorf("POST /versions: HTTP %d: %s", st, respBody)
	}
	log.Printf("deploying %s@%s …", project, version)
	if err := waitVersionReady(api, token, project, version, 2*time.Minute); err != nil {
		return err
	}
	gw := strings.TrimRight(envOr("GATEWAY_URL", "http://127.0.0.1:8787"), "/")
	log.Printf("ready %s@%s → %s/%s/%s/", project, version, gw, project, version)
	return nil
}

func apiJSON(method, url, token string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func waitVersionReady(api, token, project, version string, d time.Duration) error {
	deadline := time.Now().Add(d)
	url := api + "/v1/projects/" + project + "/versions/" + version
	for time.Now().Before(deadline) {
		st, body, err := apiJSON(http.MethodGet, url, token, nil)
		if err == nil && st == http.StatusOK {
			var v struct {
				Status string  `json:"status"`
				Error  *string `json:"error"`
			}
			_ = json.Unmarshal(body, &v)
			switch v.Status {
			case "ready":
				return nil
			case "failed", "destroyed":
				msg := v.Status
				if v.Error != nil && *v.Error != "" {
					msg = *v.Error
				}
				return fmt.Errorf("version %s: %s", version, msg)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s@%s to become ready", project, version)
}

func wranglerName(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "wrangler.jsonc"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(stripJSONC(raw), &cfg) != nil {
		return ""
	}
	return cfg.Name
}

func stripJSONC(b []byte) []byte {
	// wrangler.jsonc in this repo has no comments in the name-only path; pass through.
	return b
}

func copyPath(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := os.MkdirAll(dst, st.Mode()); err != nil {
			return err
		}
		ents, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range ents {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, st.Mode())
}
