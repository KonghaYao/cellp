package orch_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
)

func writeWrangler(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestD1DeployPlanForVersion(t *testing.T) {
	parentID := "v-parent"
	parent := &registry.Version{ID: parentID, Status: registry.StatusReady}
	child := &registry.Version{ParentVersionID: &parentID}

	t.Run("no parent", func(t *testing.T) {
		root := t.TempDir()
		writeWrangler(t, root, `{"d1_databases":[{"database_name":"guestbook"}]}`)
		plan, err := orch.D1DeployPlanForVersion(&registry.Version{}, parent, root)
		if err != nil {
			t.Fatal(err)
		}
		if plan.UseBranch {
			t.Fatal("expected no branch without parent")
		}
	})

	t.Run("parent not ready", func(t *testing.T) {
		root := t.TempDir()
		writeWrangler(t, root, `{"d1_databases":[{"database_name":"guestbook"}]}`)
		notReady := &registry.Version{ID: parentID, Status: registry.StatusDeploying}
		_, err := orch.D1DeployPlanForVersion(child, notReady, root)
		if err == nil {
			t.Fatal("expected error when parent not ready")
		}
	})

	t.Run("counter skip", func(t *testing.T) {
		root := t.TempDir()
		writeWrangler(t, root, `{"name":"counter"}`)
		plan, err := orch.D1DeployPlanForVersion(child, parent, root)
		if err != nil {
			t.Fatal(err)
		}
		if plan.UseBranch {
			t.Fatal("expected no branch without d1_databases")
		}
	})

	t.Run("branch when parent ready and d1", func(t *testing.T) {
		root := t.TempDir()
		writeWrangler(t, root, `{"d1_databases":[{"database_name":"guestbook","database_id":"x"}]}`)
		plan, err := orch.D1DeployPlanForVersion(child, parent, root)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.UseBranch || plan.ParentID != parentID {
			t.Fatalf("plan = %+v, want branch from %s", plan, parentID)
		}
	})
}
