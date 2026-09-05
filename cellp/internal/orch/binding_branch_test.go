package orch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestBindingBranchPlanForVersion(t *testing.T) {
	parentID := "parent"
	child := &registry.Version{ProjectID: "demo", ParentVersionID: &parentID}
	parentReady := &registry.Version{ID: "parent", Status: registry.StatusReady}
	parentPending := &registry.Version{ID: "parent", Status: registry.StatusPending}

	dir := t.TempDir()
	bundle := filepath.Join(dir, "bundle")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{"kv_namespaces":[{"binding":"KV","id":"x"}]}`
	if err := os.WriteFile(filepath.Join(bundle, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := BindingBranchPlanForVersion(child, parentReady, bundle)
	if err != nil || !plan.UseBranch || plan.ParentID != "parent" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}

	_, err = BindingBranchPlanForVersion(child, parentPending, bundle)
	if err == nil {
		t.Fatal("expected parent not ready error")
	}

	noParent := &registry.Version{ProjectID: "demo"}
	plan, err = BindingBranchPlanForVersion(noParent, nil, bundle)
	if err != nil || plan.UseBranch {
		t.Fatalf("no parent_version: plan=%+v err=%v", plan, err)
	}

	emptyBundle := filepath.Join(dir, "empty")
	if err := os.MkdirAll(emptyBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyBundle, "wrangler.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = BindingBranchPlanForVersion(child, parentReady, emptyBundle)
	if err != nil || plan.UseBranch {
		t.Fatalf("no bindings: plan=%+v err=%v", plan, err)
	}
}

func TestParentBranchable(t *testing.T) {
	if parentBranchable(nil) {
		t.Fatal("nil parent")
	}
	archived := &registry.Version{Status: registry.StatusArchived}
	if !parentBranchable(archived) {
		t.Fatal("archived should branch")
	}
	deployReady := &registry.Version{Status: registry.StatusDeployReady}
	if !parentBranchable(deployReady) {
		t.Fatal("deploy_ready parent should branch (AD-15)")
	}
}
