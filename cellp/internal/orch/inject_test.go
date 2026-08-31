package orch

import (
	"os"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestShouldInjectFailure(t *testing.T) {
	t.Setenv("CELLP_E2E_INJECT_DEPLOY_FAIL", "1")
	if !shouldInjectFailure(&registry.Version{}) {
		t.Fatal("env inject")
	}
	t.Setenv("CELLP_E2E_INJECT_DEPLOY_FAIL", "")
	if !shouldInjectFailure(&registry.Version{GitSHA: "fail"}) {
		t.Fatal("sha fail")
	}
	if !shouldInjectFailure(&registry.Version{GitRef: "feature-fail-deploy"}) {
		t.Fatal("ref fail")
	}
	if shouldInjectFailure(&registry.Version{GitRef: "main", GitSHA: "ok"}) {
		t.Fatal("should not inject")
	}
	_ = os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL")
}
