package orch

import (
	"context"
	"fmt"

	"github.com/cellp/cellp/internal/registry"
)

func (o *Orchestrator) ensurePreviewIngress(ctx context.Context, projectID, versionID string) (previewHost, previewURL string, err error) {
	previewHost = o.cfg.PreviewHost(projectID, versionID)
	previewURL = o.cfg.FormatPreviewURL(previewHost, nil)
	if previewURL == "" {
		return "", "", fmt.Errorf("preview url empty")
	}
	synthetic := o.cfg.PreviewSyntheticHost(projectID, versionID)
	vid := versionID
	binding := registry.IngressBinding{
		BindingID:     fmt.Sprintf("%s-%s-preview", projectID, versionID),
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
