package config

import "testing"

func TestStripPlatformEnv(t *testing.T) {
	got := StripPlatformEnv(map[string]string{
		"GREETING":        "hi",
		"PROJECT_ID":      "evil",
		"VERSION_ID":      "evil",
		"CELLP_ADMIN":     "x",
		"CELLD_VARS_FILE": "/tmp/x",
		"CELLD_REGISTRY":  "x",
	})
	if len(got) != 1 || got["GREETING"] != "hi" {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizeWorkerEnvRejectsBadKey(t *testing.T) {
	_, err := NormalizeWorkerEnv(map[string]string{"not-valid": "x"})
	if err == nil {
		t.Fatal("expected invalid key")
	}
}
