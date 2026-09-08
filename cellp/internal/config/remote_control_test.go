package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRemoteAllowlist(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "nodes.json")
	body := `[{"node_id":"n1","identity_uri":"spiffe://cellp/test/node/n1","agent_base_url":"https://127.0.0.1:19444","allow_loopback":true}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func setFullRemoteControlEnv(t *testing.T, dir string, allowlistPath string) {
	t.Helper()
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", "127.0.0.1:19445")
	t.Setenv("CELLP_CONTROLLER_IDENTITY_URI", "spiffe://cellp/test/controller/ctrl-1")
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_CERT_FILE", filepath.Join(dir, "server.pem"))
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_KEY_FILE", filepath.Join(dir, "server-key.pem"))
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_CA_FILE", filepath.Join(dir, "ca.pem"))
	t.Setenv("CELLP_CONTROLLER_NODE_ALLOWLIST_FILE", allowlistPath)
	t.Setenv("CELLP_CONTROLLER_REMOTE_NODE_HEARTBEAT_TTL", "30s")
	t.Setenv("CELLP_CONTROLLER_REMOTE_MAX_BODY_BYTES", "1048576")
	t.Setenv("CELLP_CONTROLLER_REMOTE_REPLAY_MAX_ENTRIES", "4096")
}

func TestLoadRemoteControlElasticOffIgnoresEnv(t *testing.T) {
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", "127.0.0.1:1")
	cfg, err := LoadRemoteControlConfig(false)
	if err != nil || cfg.Enabled {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlNoEnvQuiescent(t *testing.T) {
	cfg, err := LoadRemoteControlConfig(true)
	if err != nil || cfg.Enabled {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlPartialFailsClosed(t *testing.T) {
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", "127.0.0.1:19445")
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected fail closed cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlValid(t *testing.T) {
	dir := t.TempDir()
	allow := writeRemoteAllowlist(t, dir)
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, allow)
	cfg, err := LoadRemoteControlConfig(true)
	if err != nil || !cfg.Enabled || len(cfg.NodeAllowlist) != 1 {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	if cfg.NodeAllowlist["n1"].IdentityURI == "" {
		t.Fatal("expected binding")
	}
}

func TestLoadRemoteControlRejectsPublicBind(t *testing.T) {
	dir := t.TempDir()
	allow := writeRemoteAllowlist(t, dir)
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, allow)
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", "8.8.8.8:19445")
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected rejection cfg=%+v err=%v", cfg, err)
	}
	if !strings.Contains(err.Error(), "bind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRemoteControlRejectsWildcardBind(t *testing.T) {
	dir := t.TempDir()
	allow := writeRemoteAllowlist(t, dir)
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, allow)
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", "0.0.0.0:19445")
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected rejection cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlRejectsInvalidControllerSPIFFE(t *testing.T) {
	dir := t.TempDir()
	allow := writeRemoteAllowlist(t, dir)
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, allow)
	t.Setenv("CELLP_CONTROLLER_IDENTITY_URI", "https://not-spiffe/controller")
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected rejection cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlRejectsMalformedAllowlistJSON(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, bad)
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected rejection cfg=%+v err=%v", cfg, err)
	}
	if strings.Contains(err.Error(), "not-json") {
		t.Fatalf("error leaked file contents: %v", err)
	}
}

func TestLoadRemoteControlRejectsDuplicateNodeIDInAllowlistFile(t *testing.T) {
	dir := t.TempDir()
	dup := filepath.Join(dir, "dup.json")
	body := `[{"node_id":"n1","identity_uri":"spiffe://cellp/test/node/n1","agent_base_url":"https://127.0.0.1:19444","allow_loopback":true},{"node_id":"n1","identity_uri":"spiffe://cellp/test/node/n2","agent_base_url":"https://127.0.0.1:19445","allow_loopback":true}]`
	if err := os.WriteFile(dup, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, dup)
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected duplicate rejection cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRemoteControlRejectsDuplicateSPIFFEInAllowlistFile(t *testing.T) {
	dir := t.TempDir()
	dup := filepath.Join(dir, "dup-uri.json")
	body := `[{"node_id":"n1","identity_uri":"spiffe://cellp/test/node/same","agent_base_url":"https://127.0.0.1:19444","allow_loopback":true},{"node_id":"n2","identity_uri":"spiffe://cellp/test/node/same","agent_base_url":"https://127.0.0.1:19445","allow_loopback":true}]`
	if err := os.WriteFile(dup, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"server.pem": "/cert", "server-key.pem": "/key", "ca.pem": "/ca",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setFullRemoteControlEnv(t, dir, dup)
	cfg, err := LoadRemoteControlConfig(true)
	if err == nil || cfg.Enabled {
		t.Fatalf("expected duplicate uri rejection cfg=%+v err=%v", cfg, err)
	}
}

func TestRemoteControlErrorsDoNotContainFileContents(t *testing.T) {
	dir := t.TempDir()
	allow := writeRemoteAllowlist(t, dir)
	setFullRemoteControlEnv(t, dir, allow)
	t.Setenv("CELLP_CONTROLLER_IDENTITY_URI", "")
	_, err := LoadRemoteControlConfig(true)
	if err == nil || strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), allow) {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}
