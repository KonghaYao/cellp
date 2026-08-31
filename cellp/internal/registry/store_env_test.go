package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestVersionEnvRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "env.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"K": "V", "X": "Y"}); err != nil {
		t.Fatal(err)
	}
	env, err := store.GetVersionEnv(ctx, "demo", "v1")
	if err != nil || env["K"] != "V" {
		t.Fatalf("env=%v err=%v", env, err)
	}
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	env, _ = store.GetVersionEnv(ctx, "demo", "v1")
	if len(env) != 0 {
		t.Fatalf("cleared env=%v", env)
	}
}
