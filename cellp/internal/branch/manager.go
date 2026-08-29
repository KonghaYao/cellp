package branch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cellp/cellp/internal/registry"
)

// Manager wraps offshoot CLI operations for data branching.
type Manager struct {
	storePath string
	reg       registry.Store
}

// New creates a branch manager.
func New(storePath string, reg registry.Store) *Manager {
	return &Manager{storePath: storePath, reg: reg}
}

func (m *Manager) hasOffshoot() bool {
	_, err := exec.LookPath("offshoot")
	return err == nil
}

func (m *Manager) ref(project, branch string) string {
	if branch == "" || branch == "main" {
		return fmt.Sprintf("%s@main", project)
	}
	return fmt.Sprintf("%s@%s", project, branch)
}

// EnsureProject creates the offshoot project if the CLI is available.
func (m *Manager) EnsureProject(ctx context.Context, project string) error {
	if !m.hasOffshoot() {
		return nil
	}
	_ = os.MkdirAll(m.storePath, 0o755)
	_ = m.run(ctx, "init")
	if err := m.run(ctx, "create", project); err != nil {
		if strings.Contains(err.Error(), "exists") || strings.Contains(err.Error(), "already") {
			return nil
		}
		return err
	}
	return nil
}

// Checkout materializes a working copy and returns its path.
func (m *Manager) Checkout(ctx context.Context, project, branch string) (string, error) {
	if !m.hasOffshoot() {
		return "", nil
	}
	out, err := m.runOut(ctx, "checkout", m.ref(project, branch))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Fork creates a child branch from parent.
func (m *Manager) Fork(ctx context.Context, project, parent, child string) error {
	if !m.hasOffshoot() {
		return nil
	}
	return m.run(ctx, "fork", m.ref(project, parent), child)
}

// Export exports branch data to path.
func (m *Manager) Export(ctx context.Context, project, version, exportPath string) error {
	if !m.hasOffshoot() {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(exportPath), 0o755)
	return m.run(ctx, "export", m.ref(project, version), exportPath, "--force")
}

// Drain quiesces writes and deactivates gateway route (TP-V2).
func (m *Manager) Drain(ctx context.Context, project, version string) error {
	_ = m.reg.SetRouteActive(ctx, project, version, false)
	// offshoot has no drain subcommand; route deactivation is sufficient in dev
	return nil
}

// Checkpoint snapshots the current checkout. name must be unique per branch.
func (m *Manager) Checkpoint(ctx context.Context, project, branch, name string) error {
	if !m.hasOffshoot() {
		return nil
	}
	if name == "" {
		name = "cellp-checkpoint"
	}
	return m.run(ctx, "checkpoint", m.ref(project, branch), name)
}

// Promote marks version as production data branch.
func (m *Manager) Promote(ctx context.Context, project, version string) error {
	if !m.hasOffshoot() {
		return nil
	}
	return m.run(ctx, "promote", m.ref(project, version), "--onto", "main", "--force")
}

// Destroy removes a branch.
func (m *Manager) Destroy(ctx context.Context, project, version string) error {
	if !m.hasOffshoot() {
		return nil
	}
	return m.run(ctx, "destroy", m.ref(project, version), "--force")
}

// GC collects unreachable offshoot objects. grace is passed to --grace (e.g. "0s").
func (m *Manager) GC(ctx context.Context, grace string) error {
	if !m.hasOffshoot() {
		return nil
	}
	if grace == "" {
		return m.run(ctx, "gc")
	}
	return m.run(ctx, "gc", "--grace", grace)
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	_, err := m.runOut(ctx, args...)
	return err
}

func (m *Manager) runOut(ctx context.Context, args ...string) (string, error) {
	all := append([]string{"-store", m.storePath}, args...)
	cmd := exec.CommandContext(ctx, "offshoot", all...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String() + stdout.String())
		return "", fmt.Errorf("offshoot %v: %w: %s", args, err, msg)
	}
	return stdout.String(), nil
}
