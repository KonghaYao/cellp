package config

import (
	"fmt"
	"testing"
)

func TestLoadAndAddrs(t *testing.T) {
	t.Setenv("PLATFORM_PORT", "18890")
	t.Setenv("GATEWAY_PORT", "18887")
	cfg := Load()
	if cfg.APIPort != 18890 || cfg.GatewayPort != 18887 {
		t.Fatalf("ports api=%d gw=%d", cfg.APIPort, cfg.GatewayPort)
	}
	if cfg.APIAddr() != ":18890" || cfg.GatewayAddr() != ":18887" {
		t.Fatalf("addrs")
	}
}

func TestNormalizeWorkerEnvLimits(t *testing.T) {
	env := make(map[string]string)
	for i := 0; i < maxWorkerEnvKeys+1; i++ {
		env[fmt.Sprintf("K%d", i)] = "v"
	}
	_, err := NormalizeWorkerEnv(env)
	if err == nil {
		t.Fatal("expected too many keys")
	}
	_, err = NormalizeWorkerEnv(map[string]string{"OK": "bad\nvalue"})
	if err == nil {
		t.Fatal("expected newline in value")
	}
}

func TestIsPlatformEnvKey(t *testing.T) {
	if !isPlatformEnvKey("CELLP_FOO") || !isPlatformEnvKey("PROJECT_ID") || !isPlatformEnvKey("") {
		t.Fatal("platform keys")
	}
	if isPlatformEnvKey("GREETING") {
		t.Fatal("user key")
	}
}
