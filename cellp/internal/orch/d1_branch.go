package orch

import (
	"fmt"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

type D1DeployPlan struct {
	UseBranch bool
	ParentID  string
}

// D1DeployPlanForVersion decides whether a child deploy should skip export and call D1Branch.
func D1DeployPlanForVersion(v *registry.Version, parent *registry.Version, childBundleDir string) (D1DeployPlan, error) {
	if v.ParentVersionID == nil || *v.ParentVersionID == "" {
		return D1DeployPlan{}, nil
	}
	database, err := runtime.D1DatabaseName(childBundleDir)
	if err != nil {
		return D1DeployPlan{}, err
	}
	if database == "" {
		return D1DeployPlan{}, nil
	}
	if parent == nil {
		return D1DeployPlan{}, fmt.Errorf("parent version %q not found", *v.ParentVersionID)
	}
	if parent.Status != registry.StatusReady {
		return D1DeployPlan{}, fmt.Errorf("parent version %s not ready: %s", *v.ParentVersionID, parent.Status)
	}
	return D1DeployPlan{UseBranch: true, ParentID: *v.ParentVersionID}, nil
}
