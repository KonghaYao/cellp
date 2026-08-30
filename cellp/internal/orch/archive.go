package orch

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

// ArchiveConfig controls idle archive policy (AD-9).
type ArchiveConfig struct {
	Grace          time.Duration
	Idle           time.Duration
	RollbackKeep   time.Duration
	ReaperInterval time.Duration
}

// LoadArchiveConfig reads archive env vars.
func LoadArchiveConfig() ArchiveConfig {
	cfg := ArchiveConfig{
		Grace:          15 * time.Minute,
		Idle:           45 * time.Minute,
		RollbackKeep:   60 * time.Minute,
		ReaperInterval: time.Minute,
	}
	if v := os.Getenv("CELLP_ARCHIVE_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Grace = d
		}
	}
	if v := os.Getenv("CELLP_ARCHIVE_IDLE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Idle = d
		}
	}
	if v := os.Getenv("CELLP_ROLLBACK_KEEP"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.RollbackKeep = d
		}
	}
	if v := os.Getenv("CELLP_ARCHIVE_REAPER"); v != "" {
		if v == "0" {
			cfg.ReaperInterval = 0
		} else if d, err := time.ParseDuration(v); err == nil {
			cfg.ReaperInterval = d
		}
	}
	return cfg
}

// MayArchive reports whether a ready version is eligible for idle archive.
func MayArchive(proj *registry.Project, v *registry.Version, cfg ArchiveConfig, now time.Time) bool {
	if proj == nil || v == nil || v.Status != registry.StatusReady {
		return false
	}
	if proj.ProdVersionID != nil && *proj.ProdVersionID == v.ID {
		return false
	}
	if v.Pinned {
		return false
	}
	if v.ReadyAt != nil && now.Sub(*v.ReadyAt) < cfg.Grace {
		return false
	}
	if proj.PreviousProdVersionID != nil && *proj.PreviousProdVersionID == v.ID {
		if proj.PreviousProdAt != nil && now.Sub(*proj.PreviousProdAt) < cfg.RollbackKeep {
			return false
		}
	}
	last := lastAccessTime(v)
	if now.Sub(last) < cfg.Idle {
		return false
	}
	return true
}

func lastAccessTime(v *registry.Version) time.Time {
	if v.LastAccessAt != nil {
		return *v.LastAccessAt
	}
	if v.ReadyAt != nil {
		return *v.ReadyAt
	}
	return v.UpdatedAt
}

// Archive stops celld for a ready version and marks it archived (S3/offshoot retained).
func (o *Orchestrator) Archive(ctx context.Context, projectID, versionID string) error {
	v, err := o.store.GetVersion(ctx, projectID, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if v.Status != registry.StatusReady {
		return fmt.Errorf("version not ready: %s", v.Status)
	}
	proj, err := o.store.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return fmt.Errorf("project not found")
	}
	if proj.ProdVersionID != nil && *proj.ProdVersionID == versionID {
		return fmt.Errorf("cannot archive prod version")
	}
	if v.Pinned {
		return fmt.Errorf("version is pinned")
	}

	_ = o.store.UpdateVersionStatus(ctx, projectID, versionID, registry.StatusDraining, nil)
	_ = o.store.SetRouteActive(ctx, projectID, versionID, false)
	_ = o.runtime.Stop(ctx, projectID, versionID)
	return o.store.UpdateVersionStatus(ctx, projectID, versionID, registry.StatusArchived, nil)
}

// Wake restarts an archived version.
func (o *Orchestrator) Wake(ctx context.Context, projectID, versionID string) error {
	v, err := o.store.GetVersion(ctx, projectID, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if v.Status == registry.StatusDestroyed || v.Status == registry.StatusFailed {
		return fmt.Errorf("cannot wake %s version", v.Status)
	}
	if v.Status != registry.StatusArchived {
		return fmt.Errorf("version not archived: %s", v.Status)
	}

	host, port, err := o.runtime.Start(ctx, projectID, versionID)
	if err != nil {
		return fmt.Errorf("start celld: %w", err)
	}
	if !o.runtime.Health(ctx, host, port) {
		return fmt.Errorf("health check failed")
	}
	if err := o.store.SetRoute(ctx, registry.Route{
		ProjectID: projectID, VersionID: versionID, Active: true,
		UpstreamHost: host, UpstreamPort: port,
	}); err != nil {
		return err
	}
	if err := o.store.UpdateVersionStatus(ctx, projectID, versionID, registry.StatusReady, nil); err != nil {
		return err
	}
	return o.store.TouchLastAccess(ctx, projectID, versionID)
}

// RunArchiveReaperOnce archives idle-ready versions that pass MayArchive.
func (o *Orchestrator) RunArchiveReaperOnce(ctx context.Context, cfg ArchiveConfig) (int, error) {
	versions, err := o.store.ListAllReadyVersions(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	archived := 0
	projectCache := make(map[string]*registry.Project)
	for _, v := range versions {
		proj, ok := projectCache[v.ProjectID]
		if !ok {
			var perr error
			proj, perr = o.store.GetProject(ctx, v.ProjectID)
			if perr != nil || proj == nil {
				continue
			}
			projectCache[v.ProjectID] = proj
		}
		if !MayArchive(proj, &v, cfg, now) {
			continue
		}
		if err := o.Archive(ctx, v.ProjectID, v.ID); err != nil {
			log.Printf("orch: archive reaper %s/%s: %v", v.ProjectID, v.ID, err)
			continue
		}
		archived++
	}
	return archived, nil
}

// StartArchiveReaper runs idle archive on a ticker (CELLP_ARCHIVE_REAPER=0 disables).
func (o *Orchestrator) StartArchiveReaper(ctx context.Context, cfg ArchiveConfig) {
	if cfg.ReaperInterval <= 0 {
		log.Println("orch: archive reaper disabled (CELLP_ARCHIVE_REAPER=0)")
		return
	}
	go func() {
		log.Printf("orch: archive reaper every %v (idle=%v grace=%v rollback_keep=%v)",
			cfg.ReaperInterval, cfg.Idle, cfg.Grace, cfg.RollbackKeep)
		ticker := time.NewTicker(cfg.ReaperInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := o.RunArchiveReaperOnce(ctx, cfg)
				if err != nil {
					log.Printf("orch: archive reaper: %v", err)
				} else if n > 0 {
					log.Printf("orch: archive reaper archived %d version(s)", n)
				}
			}
		}
	}()
}

// ParseRollbackKeepForTest exposes rollback keep parsing for tests.
func ParseRollbackKeepForTest() time.Duration {
	if v := os.Getenv("CELLP_ROLLBACK_KEEP"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 60 * time.Minute
}
