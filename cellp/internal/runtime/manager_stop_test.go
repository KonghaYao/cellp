package runtime

import (
	"context"
	"testing"
)

func TestStopAfterStartWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	if _, _, err := m.Start(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestSetWorkerEnvLoader(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	m.SetWorkerEnvLoader(func(ctx context.Context, project, version string) (map[string]string, error) {
		return map[string]string{"GREETING": "hi"}, nil
	})
	if m == nil {
		t.Fatal("nil manager")
	}
}
