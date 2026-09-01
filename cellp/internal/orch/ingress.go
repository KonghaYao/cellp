package orch

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
)

func previewBindingID(projectID, versionID string) string {
	return fmt.Sprintf("%s-%s-preview", projectID, versionID)
}

func prodBindingID(projectID string) string {
	return fmt.Sprintf("%s-prod", projectID)
}

func (o *Orchestrator) effectiveTier(ctx context.Context, projectID string) (string, error) {
	proj, err := o.store.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	var override *string
	if proj != nil {
		override = proj.IngressTierB
	}
	global := o.cfg.Ingress.TierBOrDefault()
	return config.EffectiveIngressTierB(global, override), nil
}

func previewUsesEphemeralPort(tier string) bool {
	return tier == config.IngressTierDedicatedPort
}

func prodUsesStableListenPort(tier string) bool {
	return tier == config.IngressTierDedicatedPort || tier == config.IngressTierProdPort
}

func (o *Orchestrator) gatewayInstanceIDPtr() *string {
	id := strings.TrimSpace(o.cfg.InstanceID)
	if id == "" {
		id = strings.TrimSpace(os.Getenv("CELLPD_INSTANCE_ID"))
	}
	if id == "" {
		return nil
	}
	return &id
}

func (o *Orchestrator) ensurePreviewIngress(ctx context.Context, projectID, versionID string) (previewHost, previewURL string, err error) {
	tier, err := o.effectiveTier(ctx, projectID)
	if err != nil {
		return "", "", err
	}
	previewHost = o.cfg.PreviewHost(projectID, versionID)
	synthetic := o.cfg.PreviewSyntheticHost(projectID, versionID)
	vid := versionID
	bid := previewBindingID(projectID, versionID)
	binding := registry.IngressBinding{
		BindingID:     bid,
		ProjectID:     projectID,
		VersionID:     &vid,
		Role:          registry.IngressRolePreview,
		Host:          &previewHost,
		SyntheticHost: synthetic,
		OwnerGatewayID: o.gatewayInstanceIDPtr(),
		Active:        true,
	}

	var listenPort *int
	if previewUsesEphemeralPort(tier) {
		pid := projectID
		if err := o.store.AttachIngressListenPort(ctx, binding, registry.AllocateIngressListenPortInput{
			Stability: registry.PortStabilityEphemeral,
			OwnerKind: registry.PortOwnerIngressBinding,
			OwnerID:   bid,
			ProjectID: &pid,
			GatewayID: o.gatewayInstanceIDPtr(),
		}); err != nil {
			return "", "", err
		}
		got, err := o.store.GetIngressBinding(ctx, bid)
		if err != nil || got == nil || got.ListenPort == nil {
			return "", "", fmt.Errorf("preview attach: binding listen_port missing")
		}
		listenPort = got.ListenPort
		previewURL = o.cfg.FormatPreviewURL("", listenPort)
	} else {
		if err := o.store.UpsertIngressBinding(ctx, binding); err != nil {
			return "", "", err
		}
		previewURL = o.cfg.FormatPreviewURL(previewHost, nil)
	}
	if previewURL == "" {
		return "", "", fmt.Errorf("preview url empty")
	}
	if err := o.store.SetVersionPreviewURL(ctx, projectID, versionID, previewURL); err != nil {
		return "", "", err
	}
	if err := o.mergeWorkerEnvKey(ctx, projectID, versionID, "PUBLIC_BASE_URL", previewURL); err != nil {
		return "", "", err
	}
	o.reconcileIngressListenersLog(ctx)
	return previewHost, previewURL, nil
}

// ensureProdIngress registers the stable prod binding (AD-12). Prod listen_port is immutable after first attach (R-PROD-PORT-STABLE).
func (o *Orchestrator) ensureProdIngress(ctx context.Context, projectID string) error {
	tier, err := o.effectiveTier(ctx, projectID)
	if err != nil {
		return err
	}
	bid := prodBindingID(projectID)
	existing, _ := o.store.GetIngressBinding(ctx, bid)

	host := o.cfg.ProdHost(projectID)
	synthetic := o.cfg.ProdSyntheticHost(projectID)
	h := host
	binding := registry.IngressBinding{
		BindingID:      bid,
		ProjectID:      projectID,
		VersionID:      nil,
		Role:           registry.IngressRoleProd,
		Host:           &h,
		SyntheticHost:  synthetic,
		OwnerGatewayID: o.gatewayInstanceIDPtr(),
		Active:         true,
	}

	if existing != nil && existing.ListenPort != nil {
		binding.ListenPort = existing.ListenPort
		if existing.OwnerGatewayID != nil {
			binding.OwnerGatewayID = existing.OwnerGatewayID
		}
		err := o.store.UpsertIngressBinding(ctx, binding)
		if err != nil {
			return err
		}
		o.reconcileIngressListenersLog(ctx)
		return nil
	}

	if !prodUsesStableListenPort(tier) {
		err := o.store.UpsertIngressBinding(ctx, binding)
		if err != nil {
			return err
		}
		o.reconcileIngressListenersLog(ctx)
		return nil
	}

	reserveOwner := registry.ProdPortReserveOwnerID(projectID)
	if err := o.store.AdoptStableIngressPortForBinding(ctx, binding, reserveOwner); err != nil {
		return err
	}
	o.reconcileIngressListenersLog(ctx)
	return nil
}

func (o *Orchestrator) teardownPreviewIngress(ctx context.Context, projectID, versionID, reason string) {
	bid := previewBindingID(projectID, versionID)
	_ = o.store.DetachIngressListenPort(ctx, bid, reason)
	_ = o.store.SetIngressBindingActive(ctx, bid, false)
	o.reconcileIngressListenersLog(ctx)
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
	prodURL, err := o.resolveProdURL(ctx, projectID)
	if err != nil {
		return err
	}
	return o.mergeWorkerEnvKey(ctx, projectID, versionID, "PUBLIC_BASE_URL", prodURL)
}

func (o *Orchestrator) resolveProdURL(ctx context.Context, projectID string) (string, error) {
	b, err := o.store.GetIngressBinding(ctx, prodBindingID(projectID))
	if err != nil {
		return "", err
	}
	var port *int
	if b != nil {
		port = b.ListenPort
	}
	return o.cfg.ProdURL(projectID, port), nil
}
