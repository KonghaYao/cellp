package orch

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

const (
	maxReadyVersionsDefault = 5
	jobLease                = 5 * time.Minute
)

// Orchestrator drives version lifecycle state machine (DESIGN §2.5).
type Orchestrator struct {
	store    registry.Store
	queue    *job.SQLiteQueue
	branch   *branch.Manager
	runtime  *runtime.Manager
	artifact *artifact.Store
	cfg      config.Config
	workerID string
}

// New creates an orchestrator.
func New(store registry.Store, q *job.SQLiteQueue, bm *branch.Manager, rm *runtime.Manager, as *artifact.Store, cfg config.Config) *Orchestrator {
	return &Orchestrator{
		store:    store,
		queue:    q,
		branch:   bm,
		runtime:  rm,
		artifact: as,
		cfg:      cfg,
		workerID: fmt.Sprintf("worker-%d", os.Getpid()),
	}
}

// Run starts N worker goroutines (CELLP_ORCH_WORKERS, default 1).
func (o *Orchestrator) Run(ctx context.Context) {
	n := WorkerCount()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		workerID := o.workerID
		if n > 1 {
			workerID = fmt.Sprintf("%s-%d", o.workerID, i)
		}
		go func(wid string) {
			defer wg.Done()
			o.runWorker(ctx, wid)
		}(workerID)
	}
	wg.Wait()
}

func (o *Orchestrator) runWorker(ctx context.Context, workerID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-o.queue.Notify():
			o.processOne(ctx, workerID)
		default:
			o.processOne(ctx, workerID)
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
}

func (o *Orchestrator) processOne(ctx context.Context, workerID string) {
	j, err := o.store.ClaimJob(ctx, workerID, jobLease)
	if err != nil || j == nil {
		return
	}
	if err := o.runDeploy(ctx, j); err != nil {
		log.Printf("orch: job %s failed: %v", j.ID, err)
		msg := err.Error()
		_ = o.store.UpdateVersionStatus(ctx, j.ProjectID, j.VersionID, registry.StatusFailed, &msg)
		_ = o.store.FailJob(ctx, j.ID)
		o.compensateDeploy(ctx, j.ProjectID, j.VersionID)
		return
	}
	_ = o.store.CompleteJob(ctx, j.ID)
}

func (o *Orchestrator) runDeploy(ctx context.Context, j *registry.Job) error {
	v, err := o.store.GetVersion(ctx, j.ProjectID, j.VersionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}

	if shouldInjectFailure(v) {
		return fmt.Errorf("injected deploy failure")
	}

	ready, err := o.store.CountReadyVersions(ctx, j.ProjectID)
	if err != nil {
		return err
	}
	maxReady := maxReadyVersionsDefault
	if v := os.Getenv("CELLP_MAX_READY_VERSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxReady = n
		}
	}
	if ready >= maxReady {
		return fmt.Errorf("ready version limit exceeded")
	}

	// fetching
	if err := o.setStatus(ctx, j, registry.StatusFetching); err != nil {
		return err
	}
	destDir := filepath.Join(o.cfg.ArtifactsDir, j.ProjectID, j.VersionID)
	_, err = o.artifact.Fetch(ctx, v.ArtifactURI, v.ArtifactDigest, destDir)
	if err != nil {
		return fmt.Errorf("fetch artifact: %w", err)
	}

	// branching
	if err := o.setStatus(ctx, j, registry.StatusBranching); err != nil {
		return err
	}
	if err := o.branch.EnsureProject(ctx, j.ProjectID); err != nil {
		return err
	}
	parentBranch := "main"
	if v.ParentVersionID != nil && *v.ParentVersionID != "" {
		parentBranch = *v.ParentVersionID
	}
	if err := o.branchStep(ctx, "checkpoint", func() error {
		// Checkpoint from the store when possible. Checkout materializes a
		// full .db (100 MB seed → hundreds of MB on disk) and is only needed
		// if offshoot cannot snapshot without a working copy.
		name := "pre-fork-" + j.VersionID
		if err := o.branch.Checkpoint(ctx, j.ProjectID, parentBranch, name); err == nil {
			log.Printf("orch: offshoot checkpoint without checkout project=%s branch=%s", j.ProjectID, parentBranch)
			return nil
		} else {
			log.Printf("orch: offshoot checkpoint needs checkout: %v", err)
		}
		if _, err := o.branch.Checkout(ctx, j.ProjectID, parentBranch); err != nil {
			return err
		}
		return o.branch.Checkpoint(ctx, j.ProjectID, parentBranch, name)
	}); err != nil {
		return err
	}
	if err := o.branchStep(ctx, "fork", func() error {
		return o.branch.Fork(ctx, j.ProjectID, parentBranch, j.VersionID)
	}); err != nil {
		return err
	}

	// preparing
	if err := o.setStatus(ctx, j, registry.StatusPreparing); err != nil {
		return err
	}

	var parent *registry.Version
	if v.ParentVersionID != nil && *v.ParentVersionID != "" {
		var perr error
		parent, perr = o.store.GetVersion(ctx, j.ProjectID, *v.ParentVersionID)
		if perr != nil {
			return fmt.Errorf("parent version: %w", perr)
		}
	}
	d1Plan, err := D1DeployPlanForVersion(v, parent, destDir)
	if err != nil {
		return err
	}
	if d1Plan.UseBranch {
		parentDir := filepath.Join(o.cfg.ArtifactsDir, j.ProjectID, d1Plan.ParentID)
		parentDBID, err := runtime.D1DatabaseID(parentDir)
		if err != nil {
			return fmt.Errorf("parent database_id: %w", err)
		}
		if parentDBID == "" {
			return fmt.Errorf("parent version %s has no d1_databases database_id", d1Plan.ParentID)
		}
		if err := runtime.SetD1DatabaseID(destDir, parentDBID); err != nil {
			return fmt.Errorf("copy parent database_id: %w", err)
		}
	}

	seedPath := filepath.Join(destDir, "seed.db")
	if !d1Plan.UseBranch {
		if err := o.branchStep(ctx, "export", func() error {
			return o.branch.Export(ctx, j.ProjectID, j.VersionID, seedPath)
		}); err != nil {
			return err
		}
	} else {
		log.Printf("orch: offshoot export skipped (d1 branch from parent %s)", d1Plan.ParentID)
	}

	// deploying
	if err := o.setStatus(ctx, j, registry.StatusDeploying); err != nil {
		return err
	}
	bundleDir := filepath.Join("dev", "examples", "counter")
	if _, err := os.Stat(filepath.Join(destDir, "wrangler.jsonc")); err == nil {
		bundleDir = destDir
	} else if alt := filepath.Join(o.cfg.ArtifactsDir, "..", "examples", "counter"); alt != bundleDir {
		if _, err := os.Stat(filepath.Join(alt, "wrangler.jsonc")); err == nil {
			bundleDir = alt
		}
	}
	if abs, err := filepath.Abs(bundleDir); err == nil {
		bundleDir = abs
	}
	if err := o.runtime.Deploy(ctx, j.ProjectID, j.VersionID, bundleDir); err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	host, port, err := o.runtime.Start(ctx, j.ProjectID, j.VersionID)
	if err != nil {
		return fmt.Errorf("start celld: %w", err)
	}
	if d1Plan.UseBranch {
		t0 := time.Now()
		if err := o.runtime.D1Branch(ctx, j.ProjectID, j.VersionID, d1Plan.ParentID, bundleDir); err != nil {
			if strictOffshoot() {
				return fmt.Errorf("d1 branch: %w", err)
			}
			log.Printf("orch: d1 branch warn after %s: %v", time.Since(t0), err)
		} else {
			log.Printf("orch: d1 branch took %s", time.Since(t0))
		}
	} else if _, err := os.Stat(seedPath); err == nil {
		t0 := time.Now()
		if err := o.runtime.D1Execute(ctx, j.ProjectID, j.VersionID, bundleDir, seedPath); err != nil {
			if strictOffshoot() {
				return fmt.Errorf("d1 seed: %w", err)
			}
			log.Printf("orch: d1 seed warn after %s: %v", time.Since(t0), err)
		} else {
			log.Printf("orch: d1 seed took %s", time.Since(t0))
		}
	}

	// health gate
	if !o.runtime.Health(ctx, host, port) {
		return fmt.Errorf("health check failed")
	}

	// register route
	if err := o.store.SetRoute(ctx, registry.Route{
		ProjectID:    j.ProjectID,
		VersionID:    j.VersionID,
		Active:       true,
		UpstreamHost: host,
		UpstreamPort: port,
	}); err != nil {
		return err
	}

	if err := o.setStatus(ctx, j, registry.StatusReady); err != nil {
		return err
	}

	// Set initial prod if none
	proj, _ := o.store.GetProject(ctx, j.ProjectID)
	if proj != nil && proj.ProdVersionID == nil {
		_ = o.store.SetProdVersionCAS(ctx, j.ProjectID, "", j.VersionID)
	}
	return nil
}

func (o *Orchestrator) setStatus(ctx context.Context, j *registry.Job, status string) error {
	if err := o.store.UpdateVersionStatus(ctx, j.ProjectID, j.VersionID, status, nil); err != nil {
		return err
	}
	return o.store.UpdateJobStep(ctx, j.ID, status)
}

func strictOffshoot() bool {
	return os.Getenv("CELLP_STRICT_OFFSHOOT_FORK") == "1"
}

func (o *Orchestrator) branchStep(ctx context.Context, step string, fn func() error) error {
	t0 := time.Now()
	err := fn()
	log.Printf("orch: offshoot %s took %s err=%v", step, time.Since(t0), err)
	if err == nil {
		return nil
	}
	if strictOffshoot() {
		return fmt.Errorf("offshoot %s: %w", step, err)
	}
	log.Printf("orch: %s warn: %v", step, err)
	return nil
}

func (o *Orchestrator) compensateDeploy(ctx context.Context, projectID, versionID string) {
	_ = o.store.SetRouteActive(ctx, projectID, versionID, false)
	_ = o.branch.Destroy(ctx, projectID, versionID)
	_ = o.runtime.Stop(ctx, projectID, versionID)
}

// Promote runs the promote saga (AD-5).
func (o *Orchestrator) Promote(ctx context.Context, projectID, versionID string) error {
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

	oldProd := ""
	if proj.ProdVersionID != nil {
		oldProd = *proj.ProdVersionID
	}
	if oldProd == versionID {
		return nil
	}

	// forward saga
	var compensated []func()

	defer func() {
		if r := recover(); r != nil {
			o.runCompensation(ctx, compensated)
			panic(r)
		}
	}()

	// drain_old
	if oldProd != "" {
		if err := o.branch.Drain(ctx, projectID, oldProd); err != nil {
			return err
		}
		compensated = append(compensated, func() {
			_ = o.store.SetRouteActive(ctx, projectID, oldProd, true)
		})

		// deactivate_old_route
		if err := o.store.SetRouteActive(ctx, projectID, oldProd, false); err != nil {
			o.runCompensation(ctx, compensated)
			return err
		}
		compensated = append(compensated, func() {
			_ = o.store.SetRouteActive(ctx, projectID, oldProd, true)
		})
	}

	// offshoot_promote
	if err := o.branch.Promote(ctx, projectID, versionID); err != nil {
		log.Printf("orch: promote branch warn: %v", err)
	}
	compensated = append(compensated, func() {
		// idempotent: no reverse promote in dev
	})

	// CAS_prod
	if err := o.store.SetProdVersionCAS(ctx, projectID, oldProd, versionID); err != nil {
		o.runCompensation(ctx, compensated)
		return err
	}
	compensated = append(compensated, func() {
		_ = o.store.SetProdVersionCAS(ctx, projectID, versionID, oldProd)
	})

	// activate_prod_route
	if err := o.store.SetRouteActive(ctx, projectID, versionID, true); err != nil {
		o.runCompensation(ctx, compensated)
		return err
	}

	return nil
}

func (o *Orchestrator) runCompensation(ctx context.Context, fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

// Destroy drains and destroys a version.
func (o *Orchestrator) Destroy(ctx context.Context, projectID, versionID string) error {
	v, err := o.store.GetVersion(ctx, projectID, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if v.Status == registry.StatusDestroyed {
		return nil
	}
	if v.Status != registry.StatusReady && v.Status != registry.StatusFailed && v.Status != registry.StatusDraining {
		return fmt.Errorf("invalid status for destroy: %s", v.Status)
	}

	_ = o.store.UpdateVersionStatus(ctx, projectID, versionID, registry.StatusDraining, nil)
	_ = o.store.SetRouteActive(ctx, projectID, versionID, false)
	_ = o.runtime.Stop(ctx, projectID, versionID)
	_ = o.branch.Destroy(ctx, projectID, versionID)
	return o.store.UpdateVersionStatus(ctx, projectID, versionID, registry.StatusDestroyed, nil)
}

// ValidateForkProd rejects forking from prod for PR previews (TP-SEC-3).
func ValidateForkProd(parentVersionID *string, prodVersionID *string, gitRef string) error {
	if parentVersionID == nil || prodVersionID == nil {
		return nil
	}
	if *parentVersionID != *prodVersionID {
		return nil
	}
	ref := strings.ToLower(gitRef)
	if strings.HasPrefix(ref, "refs/pull/") || strings.HasPrefix(ref, "pr/") || strings.Contains(ref, "pull/") {
		return fmt.Errorf("cannot fork prod data for PR preview")
	}
	return nil
}

func shouldInjectFailure(v *registry.Version) bool {
	if os.Getenv("CELLP_E2E_INJECT_DEPLOY_FAIL") == "1" {
		return true
	}
	if v.GitSHA == "fail" {
		return true
	}
	ref := strings.ToLower(v.GitRef)
	return strings.Contains(ref, "fail")
}
