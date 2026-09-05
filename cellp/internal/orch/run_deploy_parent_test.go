package orch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestRunDeployChildD1AndKVBranchWithGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(gw.Close)
	installFakeCelld(t)

	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "deploy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{GatewayURL: gw.URL, ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	rm := runtime.New(freeRuntimeBasePort(t), "", "us-east-1", "s3://cellp-celld", "k", "s")
	t.Cleanup(func() { _ = rm.StopAll(context.Background()) })
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		rm,
		&artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)
	ctx := t.Context()

	parent := "v-parent"
	pid := parent
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: parent, ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v-child")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v-child", ProjectID: "demo", ParentVersionID: &pid,
		ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})

	parentDir := filepath.Join(dir, "demo", parent)
	childDir := filepath.Join(dir, "demo", "v-child")
	for _, d := range []string{parentDir, childDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	wrangler := `{"name":"app","d1_databases":[{"binding":"DB","database_name":"guestbook","database_id":"db-id-parent"}],"kv_namespaces":[{"binding":"KV","id":"ns-1"}]}`
	_ = os.WriteFile(filepath.Join(parentDir, "wrangler.jsonc"), []byte(wrangler), 0o644)
	_ = os.WriteFile(filepath.Join(childDir, "wrangler.jsonc"), []byte(wrangler), 0o644)

	j, _ := store.EnqueueJob(ctx, "demo", "v-child", registry.StatusFetching)
	if err := o.runDeploy(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v-child")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("child %+v", v)
	}
}

func TestRunDeployChildD1BranchWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	parent := "v-parent"
	pid := parent
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: parent, ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v-child")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v-child", ProjectID: "demo", ParentVersionID: &pid,
		ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})

	parentDir := filepath.Join(o.cfg.ArtifactsDir, "demo", parent)
	childDir := filepath.Join(o.cfg.ArtifactsDir, "demo", "v-child")
	for _, dir := range []string{parentDir, childDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wrangler := `{"name":"app","d1_databases":[{"binding":"DB","database_name":"guestbook","database_id":"db-id-parent"}]}`
	if err := os.WriteFile(filepath.Join(parentDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	j, _ := store.EnqueueJob(ctx, "demo", "v-child", registry.StatusFetching)
	if err := o.runDeploy(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v-child")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("child %+v", v)
	}
}
