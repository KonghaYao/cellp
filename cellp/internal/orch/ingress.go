package orch

import (
	"context"
	"fmt"

	"github.com/cellp/cellp/internal/registry"
)

func previewBindingID(projectID, versionID string) string {
	return fmt.Sprintf("%s-%s-preview", projectID, versionID)
}

func prodBindingID(projectID string) string {
	return fmt.Sprintf("%s-prod", projectID)
}

func (o *Orchestrator) ensurePreviewIngress(ctx context.Context, projectID, versionID string) (previewHost, previewURL string, err error) {
	previewHost = o.cfg.PreviewHost(projectID, versionID)
	previewURL = o.cfg.FormatPreviewURL(previewHost, nil)
	if previewURL == "" {
		return "", "", fmt.Errorf("preview url empty")
	}
	synthetic := o.cfg.PreviewSyntheticHost(projectID, versionID)
	vid := versionID
	binding := registry.IngressBinding{
		BindingID:     previewBindingID(projectID, versionID),
		ProjectID:     projectID,
		VersionID:     &vid,
		Role:          registry.IngressRolePreview,
		Host:          &previewHost,
		SyntheticHost: synthetic,
		Active:        true,
	}
	if err := o.store.UpsertIngressBinding(ctx, binding); err != nil {
		return "", "", err
	}
	if err := o.store.SetVersionPreviewURL(ctx, projectID, versionID, previewURL); err != nil {
		return "", "", err
	}
	if err := o.mergeWorkerEnvKey(ctx, projectID, versionID, "PUBLIC_BASE_URL", previewURL); err != nil {
		return "", "", err
	}
	return previewHost, previewURL, nil
}

// ensureProdIngress registers the stable prod Host binding (AD-12 P1).
func (o *Orchestrator) ensureProdIngress(ctx context.Context, projectID string) error {
	host := o.cfg.ProdHost(projectID)
	synthetic := o.cfg.ProdSyntheticHost(projectID)
	h := host
	binding := registry.IngressBinding{
		BindingID:     prodBindingID(projectID),
		ProjectID:     projectID,
		VersionID:     nil,
		Role:          registry.IngressRoleProd,
		Host:          &h,
		SyntheticHost: synthetic,
		Active:        true,
	}
	return o.store.UpsertIngressBinding(ctx, binding)
}

func (o *Orchestrator) setPreviewIngressActive(ctx context.Context, projectID, versionID string, active bool) error {
	return o.store.SetIngressBindingActive(ctx, previewBindingID(projectID, versionID), active)
}

func (o *Orchestrator) mergeWorkerEnvKey(ctx context.Context, projectID, versionID, key, value string) error {
	env, err := o.store.GetVersionEnv(ctx, projectID, versionID)
	if err != nil {
		env = map[string]string{}
	}
	if env == nil {
		env = map[string]string{}
	}
	env[key] = value
	return o.store.SetVersionEnv(ctx, projectID, versionID, env)
}

func (o *Orchestrator) mergeProdPublicBaseURL(ctx context.Context, projectID, versionID string) error {
	prodURL := o.cfg.ProdURL(projectID)
	return o.mergeWorkerEnvKey(ctx, projectID, versionID, "PUBLIC_BASE_URL", prodURL)
}
