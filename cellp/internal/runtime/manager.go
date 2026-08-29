package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

// Manager manages celld subprocess lifecycle per version (AD-1).
type Manager struct {
	basePort  int
	endpoint  string
	region    string
	bucket    string
	accessKey string
	secretKey string
	mu        sync.Mutex
	processes map[string]*celldProc
	ports     map[string]int
	nextN     int
}

type celldProc struct {
	cmd      *exec.Cmd
	port     int
	watchDir string
}

// New creates a runtime manager.
func New(basePort int, endpoint, region, bucket, accessKey, secretKey string) *Manager {
	return &Manager{
		basePort:  basePort,
		endpoint:  endpoint,
		region:    region,
		bucket:    bucket,
		accessKey: accessKey,
		secretKey: secretKey,
		processes: make(map[string]*celldProc),
		ports:     make(map[string]int),
	}
}

func (m *Manager) key(project, version string) string {
	return project + "/" + version
}

func (m *Manager) versionBucket(project, version string) string {
	return fmt.Sprintf("s3://cellp-celld/%s/%s", project, version)
}

// AllocatePort returns a unique port for a version (8792+N, skipping base dev celld).
// Ports are never reused for a different project/version while still tracked in m.ports.
func (m *Manager) AllocatePort(project, version string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(project, version)
	if p, ok := m.ports[k]; ok {
		return p
	}
	used := make(map[int]struct{}, len(m.ports))
	for _, p := range m.ports {
		used[p] = struct{}{}
	}
	for n := 1; n <= 1000; n++ {
		port := m.basePort + 10 + n
		if _, taken := used[port]; taken {
			continue
		}
		m.ports[k] = port
		if n > m.nextN {
			m.nextN = n
		}
		return port
	}
	m.nextN++
	port := m.basePort + 10 + m.nextN
	m.ports[k] = port
	return port
}

// Start launches celld on 127.0.0.1:port for the version.
func (m *Manager) Start(ctx context.Context, project, version string) (string, int, error) {
	port := m.AllocatePort(project, version)
	host := "127.0.0.1"
	k := m.key(project, version)

	m.mu.Lock()
	if p, ok := m.processes[k]; ok && p.cmd != nil && p.cmd.Process != nil {
		m.mu.Unlock()
		return host, p.port, nil
	}
	m.mu.Unlock()

	if os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL") == "1" {
		return host, port, nil
	}

	if !CelldInstalled() {
		m.mu.Lock()
		m.processes[k] = &celldProc{port: port}
		m.mu.Unlock()
		return host, port, nil
	}

	bucket := m.versionBucket(project, version)
	args := []string{
		"--bucket", bucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
		"--listen", fmt.Sprintf("127.0.0.1:%d", port),
	}
	cmd := exec.CommandContext(ctx, "celld", args...)
	watch, err := m.allocateWatchDir(project, version)
	if err != nil {
		return "", 0, fmt.Errorf("allocate watch dir: %w", err)
	}
	gateMs := os.Getenv("CELLD_READY_FLEET_GATE_MS")
	if gateMs == "" {
		// Per-version bucket is a one-node fleet. The 120s default withholds
		// health while a dead peer lease lingers; that is the wrong default
		// for AD-1 start.
		gateMs = "5000"
	}
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
		fmt.Sprintf("CELLD_WATCH=%s", watch),
		fmt.Sprintf("CELLD_READY_FLEET_GATE_MS=%s", gateMs),
	)
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("celld-%s-%s.log", project, version))
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("start celld: %w", err)
	}

	m.mu.Lock()
	m.processes[k] = &celldProc{cmd: cmd, port: port, watchDir: watch}
	m.mu.Unlock()

	for i := 0; i < 60; i++ {
		if m.Health(ctx, host, port) {
			return host, port, nil
		}
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return host, port, fmt.Errorf("celld health timeout on %s:%d", host, port)
}

// CelldInstalled reports whether the celld binary is on PATH.
func CelldInstalled() bool {
	_, err := exec.LookPath("celld")
	return err == nil
}

// Diagnose runs celld storage probe for a version bucket before deploy/start.
func (m *Manager) Diagnose(ctx context.Context, project, version string) error {
	if !CelldInstalled() {
		return nil
	}
	bucket := m.versionBucket(project, version)
	cmd := exec.CommandContext(ctx, "celld", "diagnose",
		"--bucket", bucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld diagnose: %w: %s", err, string(out))
	}
	return nil
}

// RuntimeRouteHealth is per-route celld upstream status.
type RuntimeRouteHealth struct {
	ProjectID    string `json:"project_id"`
	VersionID    string `json:"version_id"`
	UpstreamHost string `json:"upstream_host"`
	UpstreamPort int    `json:"upstream_port"`
	Healthy      bool   `json:"healthy"`
}

// RuntimeHealth probes each active route's celld upstream.
func (m *Manager) RuntimeHealth(ctx context.Context, routes []registry.Route) []RuntimeRouteHealth {
	out := make([]RuntimeRouteHealth, 0, len(routes))
	for _, r := range routes {
		out = append(out, RuntimeRouteHealth{
			ProjectID:    r.ProjectID,
			VersionID:    r.VersionID,
			UpstreamHost: r.UpstreamHost,
			UpstreamPort: r.UpstreamPort,
			Healthy:      m.Health(ctx, r.UpstreamHost, r.UpstreamPort),
		})
	}
	return out
}

// Deploy runs celld deploy for a bundle directory.
func (m *Manager) Deploy(ctx context.Context, project, version, exampleDir string) error {
	if os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL") == "1" {
		return fmt.Errorf("injected deploy failure")
	}
	if !CelldInstalled() {
		return nil
	}
	if err := m.Diagnose(ctx, project, version); err != nil {
		return err
	}
	bucket := m.versionBucket(project, version)
	cmd := exec.CommandContext(ctx, "celld", "deploy", exampleDir,
		"--bucket", bucket, "--endpoint", m.endpoint, "--region", m.region)
	env := append(os.Environ(),
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)
	if esbuild := findEsbuild(exampleDir); esbuild != "" {
		env = append(env, "CELLD_ESBUILD="+esbuild)
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld deploy: %w: %s", err, string(out))
	}
	return nil
}

func findEsbuild(exampleDir string) string {
	if v := os.Getenv("CELLD_ESBUILD"); v != "" {
		return v
	}
	candidates := []string{
		filepath.Join(exampleDir, "node_modules", ".bin", "esbuild"),
		filepath.Join("dev", "examples", "counter", "node_modules", ".bin", "esbuild"),
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			c = abs
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("esbuild"); err == nil {
		return p
	}
	return ""
}

// D1Branch links a child version D1 to a parent bucket baseline (celld d1 branch).
func (m *Manager) D1Branch(ctx context.Context, project, childVersion, parentVersion, projectDir string) error {
	if _, err := exec.LookPath("celld"); err != nil {
		return nil
	}
	database, err := D1DatabaseName(projectDir)
	if err != nil {
		return err
	}
	if database == "" {
		return nil
	}
	parentBucket := m.versionBucket(project, parentVersion)
	childBucket := m.versionBucket(project, childVersion)
	cmd := exec.CommandContext(ctx, "celld",
		"d1", "branch", database,
		"--parent-bucket", parentBucket,
		projectDir,
		"--bucket", childBucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", childVersion),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld d1 branch: %w: %s", err, string(out))
	}
	return nil
}

// D1DatabaseID returns the sole wrangler d1_databases[0].database_id, or "" when none.
func D1DatabaseID(projectDir string) (string, error) {
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return "", err
	}
	var cfg struct {
		D1 []struct {
			ID string `json:"database_id"`
		} `json:"d1_databases"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse wrangler: %w", err)
	}
	switch len(cfg.D1) {
	case 0:
		return "", nil
	case 1:
		return strings.TrimSpace(cfg.D1[0].ID), nil
	default:
		return "", fmt.Errorf("wrangler has %d d1_databases entries in %s; only one is supported", len(cfg.D1), projectDir)
	}
}

// SetD1DatabaseID writes database_id into the child's wrangler config.
func SetD1DatabaseID(projectDir, databaseID string) error {
	path, raw, err := readWranglerConfigFile(projectDir)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse wrangler: %w", err)
	}
	dbs, ok := cfg["d1_databases"].([]any)
	if !ok || len(dbs) == 0 {
		return fmt.Errorf("wrangler has no d1_databases in %s", projectDir)
	}
	db0, ok := dbs[0].(map[string]any)
	if !ok {
		return fmt.Errorf("wrangler d1_databases[0] is not an object in %s", projectDir)
	}
	db0["database_id"] = databaseID
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// D1Execute seeds D1 from export path when celld is available.
// Binary SQLite seeds use `celld d1 import`; plain SQL files use `d1 execute --file`.
func (m *Manager) D1Execute(ctx context.Context, project, version, projectDir, seedPath string) error {
	if _, err := exec.LookPath("celld"); err != nil {
		return nil
	}
	if _, err := os.Stat(seedPath); err != nil {
		return nil
	}
	database, err := D1DatabaseName(projectDir)
	if err != nil {
		return err
	}
	if database == "" {
		return nil
	}
	sqlite := isSQLiteFile(seedPath)
	if sqlite {
		if err := removeSQLiteSidecars(seedPath); err != nil {
			return fmt.Errorf("prepare sqlite seed: %w", err)
		}
	}
	bucket := m.versionBucket(project, version)
	var args []string
	if sqlite {
		args = []string{
			"d1", "import", database,
			"--file", seedPath,
			projectDir,
			"--bucket", bucket,
			"--endpoint", m.endpoint,
			"--region", m.region,
		}
	} else {
		args = []string{
			"d1", "execute", database,
			"--file", seedPath,
			projectDir,
			"--bucket", bucket,
			"--endpoint", m.endpoint,
			"--region", m.region,
		}
	}
	cmd := exec.CommandContext(ctx, "celld", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)
	subcmd := "execute"
	if sqlite {
		subcmd = "import"
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld d1 %s: %w: %s", subcmd, err, string(out))
	}
	return nil
}

func D1DatabaseName(projectDir string) (string, error) {
	raw, err := readWranglerConfig(projectDir)
	if err != nil {
		return "", err
	}
	var cfg struct {
		D1 []struct {
			Name string `json:"database_name"`
		} `json:"d1_databases"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse wrangler: %w", err)
	}
	switch len(cfg.D1) {
	case 0:
		return "", nil
	case 1:
		name := strings.TrimSpace(cfg.D1[0].Name)
		if name == "" {
			return "", fmt.Errorf("wrangler d1_databases[0].database_name is empty")
		}
		return name, nil
	default:
		return "", fmt.Errorf("wrangler has %d d1_databases entries in %s; only one is supported", len(cfg.D1), projectDir)
	}
}

func readWranglerConfig(projectDir string) ([]byte, error) {
	_, data, err := readWranglerConfigFile(projectDir)
	return data, err
}

func readWranglerConfigFile(projectDir string) (string, []byte, error) {
	for _, name := range []string{"wrangler.jsonc", "wrangler.json"} {
		path := filepath.Join(projectDir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			if strings.HasSuffix(name, ".jsonc") {
				data = []byte(stripJSONC(string(data)))
			}
			return path, data, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("%w: no wrangler.jsonc or wrangler.json in %s", ErrNoWrangler, projectDir)
}

func stripJSONC(src string) string {
	var out strings.Builder
	out.Grow(len(src))
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		out.WriteByte(src[i])
		i++
	}
	return out.String()
}

// removeSQLiteSidecars deletes -wal/-shm siblings so celld d1 import accepts the file.
func removeSQLiteSidecars(dbPath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func isSQLiteFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [16]byte
	n, err := f.Read(hdr[:])
	if err != nil || n < 16 {
		return false
	}
	return string(hdr[:15]) == "SQLite format 3"
}

// Health checks celld /.well-known/celld/health; returns true when celld is absent (dev).
func (m *Manager) Health(ctx context.Context, host string, port int) bool {
	if !CelldInstalled() {
		return true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s:%d/.well-known/celld/health", host, port), nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// allocateWatchDir returns a working directory for celld SQLite/LTX.
// Default: ephemeral temp dir removed on Stop (S3/RustFS is durable store).
// Set CELLP_CELLD_WATCH_PERSIST=1 for legacy persistent dev/data/celld-watch paths.
func (m *Manager) allocateWatchDir(project, version string) (string, error) {
	if os.Getenv("CELLP_CELLD_WATCH_PERSIST") == "1" {
		watch := filepath.Join("dev", "data", "celld-watch", project, version)
		if abs, err := filepath.Abs(watch); err == nil {
			watch = abs
		}
		if err := os.MkdirAll(watch, 0o755); err != nil {
			return "", err
		}
		return watch, nil
	}
	root := os.Getenv("CELLP_CELLD_WATCH_TMP")
	if root == "" {
		root = os.TempDir()
	}
	prefix := fmt.Sprintf("cellp-celld-%s-%s-", sanitizeWatchToken(project), sanitizeWatchToken(version))
	return os.MkdirTemp(root, prefix)
}

func sanitizeWatchToken(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "x"
	}
	return out
}

func removeEphemeralWatch(watchDir string) {
	if watchDir == "" || os.Getenv("CELLP_CELLD_WATCH_PERSIST") == "1" {
		return
	}
	_ = os.RemoveAll(watchDir)
}

// Stop tears down a celld instance for a version.
func (m *Manager) Stop(ctx context.Context, project, version string) error {
	_ = ctx
	m.mu.Lock()
	k := m.key(project, version)
	p, ok := m.processes[k]
	if !ok || p == nil {
		delete(m.processes, k)
		m.mu.Unlock()
		return nil
	}
	watchDir := p.watchDir
	cmd := p.cmd
	delete(m.processes, k)
	m.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		removeEphemeralWatch(watchDir)
		return nil
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	case <-done:
	}
	removeEphemeralWatch(watchDir)
	return nil
}
