package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cellp/cellp/internal/health"
	"github.com/cellp/cellp/internal/registry"
)

// Manager manages celld subprocess lifecycle per version (AD-1).
type Manager struct {
	basePort     int
	endpoint     string
	region       string
	bucket       string // Retained for constructor compatibility; AD-1 per-version paths never derive from it.
	accessKey    string
	secretKey    string
	mu           sync.Mutex
	processes    map[string]*celldProc
	ports        map[string]int
	lifecycle    map[string]*lifecycleLock
	nextN        int
	envLoader    WorkerEnvLoader
	replicaHosts ReplicaHostConfig
}

// WorkerEnvLoader returns dashboard/CD Worker vars for a version (not platform keys).
type WorkerEnvLoader func(ctx context.Context, project, version string) (map[string]string, error)

type celldProc struct {
	cmd           *exec.Cmd
	port          int
	watchDir      string
	bindHost      string
	advertiseHost string
}

type lifecycleLock struct {
	mu   sync.Mutex
	refs int
}

// New creates a runtime manager.
func New(basePort int, endpoint, region, bucket, accessKey, secretKey string) *Manager {
	return &Manager{
		basePort:     basePort,
		endpoint:     endpoint,
		region:       region,
		bucket:       bucket,
		accessKey:    accessKey,
		secretKey:    secretKey,
		processes:    make(map[string]*celldProc),
		ports:        make(map[string]int),
		lifecycle:    make(map[string]*lifecycleLock),
		replicaHosts: DefaultReplicaHostConfig(),
	}
}

// SetWorkerEnvLoader supplies per-version Worker vars written to CELLD_VARS_FILE at Start.
func (m *Manager) SetWorkerEnvLoader(fn WorkerEnvLoader) {
	m.envLoader = fn
}

func (m *Manager) key(project, version string) string {
	return project + "/" + version
}

func (m *Manager) replicaKey(key ReplicaKey) string {
	return key.ProjectID + "/" + key.VersionID + "/replicas/" + key.ReplicaID
}

func (m *Manager) lockLifecycle(project, version string) func() {
	return m.lockLifecycleKey(m.key(project, version))
}

func (m *Manager) lockLifecycleKey(k string) func() {
	m.mu.Lock()
	lock := m.lifecycle[k]
	if lock == nil {
		lock = &lifecycleLock{}
		m.lifecycle[k] = lock
	}
	lock.refs++
	m.mu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		m.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(m.lifecycle, k)
		}
		m.mu.Unlock()
	}
}

func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (m *Manager) versionBucket(project, version string) string {
	return fmt.Sprintf("s3://cellp-celld/%s/%s", project, version)
}

// ExpectedReplicaBucket returns the canonical server-derived bucket for a replica.
func (m *Manager) ExpectedReplicaBucket(key ReplicaKey) (string, error) {
	if err := key.validate(); err != nil {
		return "", err
	}
	return m.versionBucket(key.ProjectID, key.VersionID), nil
}

// ValidateReplicaBucket rejects any wire bucket that differs from the canonical bucket.
func (m *Manager) ValidateReplicaBucket(key ReplicaKey, bucket string) error {
	expected, err := m.ExpectedReplicaBucket(key)
	if err != nil {
		return err
	}
	if bucket != expected {
		return fmt.Errorf("replica bucket does not match assignment")
	}
	return nil
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

// SeedPort registers a known upstream port for a version (fleet reconcile).
// Returns an error when port collides with a different project/version.
func (m *Manager) SeedPort(project, version string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(project, version)
	if existing, ok := m.ports[k]; ok {
		if existing == port {
			return nil
		}
		return fmt.Errorf("port already allocated for %s: %d", k, existing)
	}
	for otherK, p := range m.ports {
		if p == port && otherK != k {
			return fmt.Errorf("port %d already in use by %s", port, otherK)
		}
	}
	m.ports[k] = port
	n := port - m.basePort - 10
	if n > m.nextN {
		m.nextN = n
	}
	return nil
}

// Start launches celld on 127.0.0.1:port for the version.
func (m *Manager) Start(ctx context.Context, project, version string) (string, int, error) {
	port := m.AllocatePort(project, version)
	return m.StartOnPort(ctx, project, version, "127.0.0.1", port)
}

// Restart stops then starts celld so CELLD_VARS_FILE is re-read.
func (m *Manager) Restart(ctx context.Context, project, version string) error {
	unlock := m.lockLifecycle(project, version)
	defer unlock()

	port := m.AllocatePort(project, version)
	celldInstalled := CelldInstalled()
	if err := m.stopLocked(ctx, project, version); err != nil {
		return err
	}
	if celldInstalled {
		if err := waitForTCPPortFree("127.0.0.1", port, celldListenPortSettleDuration()); err != nil {
			return err
		}
	}
	_, _, err := m.startOnPortLocked(ctx, project, version, "127.0.0.1", port)
	return err
}

// StartOnPort launches celld on host:port for the version.
func (m *Manager) StartOnPort(ctx context.Context, project, version, host string, port int) (string, int, error) {
	unlock := m.lockLifecycle(project, version)
	defer unlock()
	return m.startOnPortLocked(ctx, project, version, host, port)
}

func (m *Manager) startOnPortLocked(ctx context.Context, project, version, host string, port int) (string, int, error) {
	return m.startManagedOnPortLocked(ctx, m.key(project, version), project, version, version, m.versionBucket(project, version), host, "", port)
}

func (m *Manager) startManagedOnPortLocked(ctx context.Context, k, project, version, watchVersion, bucket, bindHost, advertiseHost string, port int) (string, int, error) {
	returnHost := externalReplicaHost(bindHost, advertiseHost)

	m.mu.Lock()
	if p, ok := m.processes[k]; ok && processAlive(p.cmd) {
		bindHost := p.bindHost
		if bindHost == "" {
			bindHost = "127.0.0.1"
		}
		runningPort := p.port
		extHost := p.externalHost()
		m.mu.Unlock()
		if err := m.waitCelldHealthy(ctx, bindHost, runningPort); err != nil {
			return "", 0, err
		}
		return extHost, runningPort, nil
	}
	var staleWatch string
	if p, ok := m.processes[k]; ok {
		// Stale map entry after the subprocess exited. Drop it and respawn.
		staleWatch = p.watchDir
		delete(m.processes, k)
	}
	m.mu.Unlock()
	if err := removeEphemeralWatch(staleWatch); err != nil {
		return "", 0, fmt.Errorf("remove stale watch: %w", err)
	}

	if os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL") == "1" {
		return returnHost, port, nil
	}

	if !CelldInstalled() {
		m.mu.Lock()
		m.processes[k] = &celldProc{port: port, bindHost: bindHost, advertiseHost: advertiseHost}
		m.mu.Unlock()
		return returnHost, port, nil
	}
	if err := waitForTCPPortFree(bindHost, port, celldListenPortSettleDuration()); err != nil {
		return "", 0, fmt.Errorf("start celld: %w", err)
	}

	args := []string{
		"--bucket", bucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
		"--listen", celldListenFlag(bindHost, port),
	}
	// celld is a long-lived AD-1 daemon. Do not bind it to the caller
	// context — HTTP wake handlers cancel when the response is written,
	// which would SIGKILL the process and 502 the preview.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "celld", args...)
	watch, err := m.allocateWatchDir(project, watchVersion)
	if err != nil {
		return "", 0, fmt.Errorf("allocate watch dir: %w", err)
	}
	keepWatch := false
	defer func() {
		if !keepWatch {
			_ = removeEphemeralWatch(watch)
		}
	}()
	gateMs := os.Getenv("CELLD_READY_FLEET_GATE_MS")
	if gateMs == "" {
		// Per-version bucket is a one-node fleet. The 120s default withholds
		// health while a dead peer lease lingers; that is the wrong default
		// for AD-1 start.
		gateMs = "5000"
	}
	envExtra := []string{
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
		fmt.Sprintf("CELLD_WATCH=%s", watch),
		fmt.Sprintf("CELLD_READY_FLEET_GATE_MS=%s", gateMs),
		"CELLD_TRUST_FORWARDED_HEADERS=1",
	}
	if m.envLoader != nil {
		workerEnv, err := m.envLoader(ctx, project, version)
		if err != nil {
			return "", 0, fmt.Errorf("load worker env: %w", err)
		}
		if len(workerEnv) > 0 {
			varsPath := filepath.Join(watch, "celld.vars")
			if err := WriteCelldVarsFile(varsPath, workerEnv); err != nil {
				return "", 0, fmt.Errorf("write CELLD_VARS_FILE: %w", err)
			}
			envExtra = append(envExtra, "CELLD_VARS_FILE="+varsPath)
		}
	}
	cmd.Env = append(os.Environ(), envExtra...)
	logPath := celldLogPath(project, watchVersion)
	var logFile *os.File
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		logFile = f
		cmd.Stdout = f
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		_ = removeEphemeralWatch(watch)
		return "", 0, fmt.Errorf("start celld: %w", err)
	}
	if logFile != nil {
		_ = logFile.Close()
	}

	m.mu.Lock()
	m.processes[k] = &celldProc{cmd: cmd, port: port, watchDir: watch, bindHost: bindHost, advertiseHost: advertiseHost}
	m.mu.Unlock()
	keepWatch = true
	ready := false
	defer func() {
		if !ready {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = m.stopManagedLocked(cleanupCtx, k, true)
		}
	}()

	if err := m.waitCelldHealthy(ctx, bindHost, port); err != nil {
		return "", 0, err
	}
	ready = true
	return returnHost, port, nil
}

// CelldInstalled reports whether the celld binary is on PATH.
func CelldInstalled() bool {
	_, err := exec.LookPath("celld")
	return err == nil
}

// Diagnose runs celld storage probe for a version bucket before deploy/start.
func (m *Manager) Diagnose(ctx context.Context, project, version string) error {
	return m.diagnoseBucket(ctx, m.versionBucket(project, version))
}

const diagnoseTimeout = 30 * time.Second

// Default time to wait for a celld listen socket to be released after stop or before start.
const defaultCelldListenPortSettle = 45 * time.Second

func celldListenPortSettleDuration() time.Duration {
	if v := strings.TrimSpace(os.Getenv("CELLP_CELLD_PORT_SETTLE")); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil && d > 0 {
			return d
		}
	}
	return defaultCelldListenPortSettle
}

const celldHealthPollAttempts = 60

func (m *Manager) waitCelldHealthy(ctx context.Context, bindHost string, port int) error {
	if !CelldInstalled() {
		return nil
	}
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	for i := 0; i < celldHealthPollAttempts; i++ {
		if m.Health(ctx, bindHost, port) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("celld health timeout on %s:%d", bindHost, port)
}

func cappedContext(ctx context.Context, cap time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, cap)
}

func (m *Manager) diagnoseBucket(ctx context.Context, bucket string) error {
	if os.Getenv("CELLP_SKIP_CELLD_DIAGNOSE") == "1" {
		return nil
	}
	if !CelldInstalled() {
		return nil
	}
	diagnoseCtx, cancel := cappedContext(ctx, diagnoseTimeout)
	defer cancel()
	cmd := exec.CommandContext(diagnoseCtx, "celld", "diagnose",
		"--bucket", bucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld diagnose failed: %w", err)
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

func celldDeployKilled(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "signal: killed")
}

// Deploy runs celld deploy for a bundle directory. When includeCrons is false, triggers.crons
// are stripped from a temp copy of the bundle (artifact unchanged); see AD-11.
func (m *Manager) Deploy(ctx context.Context, project, version, exampleDir string, includeCrons bool) error {
	if os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL") == "1" {
		return fmt.Errorf("injected deploy failure")
	}
	if !CelldInstalled() {
		return nil
	}
	if err := m.Diagnose(ctx, project, version); err != nil {
		return err
	}
	deployDir, cleanup, err := PrepareDeployBundle(exampleDir, includeCrons)
	if err != nil {
		return err
	}
	defer cleanup()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		lastErr = withCelldDeploySlot(ctx, func(runCtx context.Context) error {
			return m.runCelldDeploy(runCtx, project, version, exampleDir, deployDir)
		})
		if lastErr == nil || !celldDeployKilled(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func (m *Manager) runCelldDeploy(ctx context.Context, project, version, exampleDir, deployDir string) error {
	bucket := m.versionBucket(project, version)
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "celld", "deploy", deployDir,
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
	if os.Getenv("CELLP_E2E_INJECT_D1_BRANCH_FAIL") == "1" {
		return fmt.Errorf("injected d1 branch failure")
	}
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
	// Probe celld health on the well-known path used by production runners.
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+addr+"/.well-known/celld/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return health.CelldHealthResponseOK(resp.StatusCode, body)
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

var removeEphemeralWatch = func(watchDir string) error {
	if watchDir == "" || os.Getenv("CELLP_CELLD_WATCH_PERSIST") == "1" {
		return nil
	}
	return os.RemoveAll(watchDir)
}

// Stop tears down a celld instance for a version.
func (m *Manager) Stop(ctx context.Context, project, version string) error {
	unlock := m.lockLifecycle(project, version)
	defer unlock()
	return m.stopLocked(ctx, project, version)
}

// StopAll tears down every celld subprocess owned by this manager.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	keys := make([]string, 0, len(m.processes))
	for k := range m.processes {
		keys = append(keys, k)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	errs := make(chan error, len(keys))
	for _, k := range keys {
		k := k
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := m.lockLifecycleKey(k)
			defer unlock()
			if err := m.stopManagedLocked(ctx, k, true); err != nil {
				errs <- fmt.Errorf("stop %s: %w", k, err)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var all []error
	for err := range errs {
		all = append(all, err)
	}
	return errors.Join(all...)
}

func (m *Manager) stopLocked(ctx context.Context, project, version string) error {
	return m.stopManagedLocked(ctx, m.key(project, version), false)
}

func (m *Manager) stopManagedLocked(ctx context.Context, k string, releasePort bool) error {
	m.mu.Lock()
	p, ok := m.processes[k]
	if !ok || p == nil {
		delete(m.processes, k)
		if releasePort {
			delete(m.ports, k)
		}
		m.mu.Unlock()
		return nil
	}
	watchDir := p.watchDir
	cmd := p.cmd
	bindHost := p.bindHost
	listenPort := p.port
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("signal celld: %w", err)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("kill celld: %w", err)
			}
			select {
			case <-done:
			case <-cleanupCtx.Done():
				return fmt.Errorf("wait for killed celld process: %w", cleanupCtx.Err())
			}
		}
	}

	if cmd != nil && cmd.Process != nil && CelldInstalled() && listenPort > 0 {
		if err := waitForTCPPortFree(bindHost, listenPort, celldListenPortSettleDuration()); err != nil {
			// Our subprocess is already gone; if the listen port is still taken,
			// another process holds it. Release local inventory anyway so elastic
			// reconcile does not wedge on stale port maps.
			if processAlive(cmd) {
				return err
			}
		}
	}

	m.mu.Lock()
	if m.processes[k] == p {
		delete(m.processes, k)
		if releasePort {
			delete(m.ports, k)
		}
	}
	m.mu.Unlock()
	return removeEphemeralWatch(watchDir)
}

func celldLogPath(project, version string) string {
	component := func(v string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(v))
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("celld-%s-%s.log", component(project), component(version)))
}

func waitForTCPPortFree(host string, port int, maxWait time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(maxWait)
	for {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return nil
		}
		_ = c.Close()
		if !time.Now().Before(deadline) {
			return fmt.Errorf("port %s still in use after %v", addr, maxWait)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
