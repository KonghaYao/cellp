package orch

import (
	"fmt"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

// BindingBranchPlan describes child binding branches from a parent version.
type BindingBranchPlan struct {
	UseBranch bool
	ParentID  string
}

func parentBranchable(parent *registry.Version) bool {
	if parent == nil {
		return false
	}
	return parent.Status == registry.StatusReady || parent.Status == registry.StatusArchived
}

// BindingBranchPlanForVersion decides whether child deploy should branch KV/R2/Queue from parent.
func BindingBranchPlanForVersion(v *registry.Version, parent *registry.Version, childBundleDir string) (BindingBranchPlan, error) {
	if v.ParentVersionID == nil || *v.ParentVersionID == "" {
		return BindingBranchPlan{}, nil
	}
	bindings, err := runtime.ParseBindings(childBundleDir)
	if err != nil {
		return BindingBranchPlan{}, err
	}
	if len(bindings.KV) == 0 && len(bindings.R2) == 0 && len(bindings.Queues) == 0 {
		return BindingBranchPlan{}, nil
	}
	if parent == nil {
		return BindingBranchPlan{}, fmt.Errorf("parent version %q not found", *v.ParentVersionID)
	}
	if !parentBranchable(parent) {
		return BindingBranchPlan{}, fmt.Errorf("parent version %s not ready or archived: %s", *v.ParentVersionID, parent.Status)
	}
	return BindingBranchPlan{UseBranch: true, ParentID: *v.ParentVersionID}, nil
}
