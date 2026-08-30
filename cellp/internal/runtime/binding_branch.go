package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func (m *Manager) branchEnv(project, childVersion string) []string {
	return []string{
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", childVersion),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	}
}

// KvBranch links a child KV namespace to a parent bucket baseline.
func (m *Manager) KvBranch(ctx context.Context, project, childVersion, parentVersion, nsID string) error {
	if _, err := exec.LookPath("celld"); err != nil {
		return fmt.Errorf("celld not installed")
	}
	parentBucket := m.versionBucket(project, parentVersion)
	childBucket := m.versionBucket(project, childVersion)
	cmd := exec.CommandContext(ctx, "celld",
		"kv", "branch", nsID,
		"--parent-bucket", parentBucket,
		"--bucket", childBucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(), m.branchEnv(project, childVersion)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld kv branch: %w: %s", err, string(out))
	}
	return nil
}

// QueueBranch links a child queue to a parent bucket baseline.
func (m *Manager) QueueBranch(ctx context.Context, project, childVersion, parentVersion, queueName string) error {
	if _, err := exec.LookPath("celld"); err != nil {
		return fmt.Errorf("celld not installed")
	}
	parentBucket := m.versionBucket(project, parentVersion)
	childBucket := m.versionBucket(project, childVersion)
	cmd := exec.CommandContext(ctx, "celld",
		"queue", "branch", queueName,
		"--parent-bucket", parentBucket,
		"--bucket", childBucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(), m.branchEnv(project, childVersion)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld queue branch: %w: %s", err, string(out))
	}
	return nil
}

// R2Branch links a child R2 binding to a parent bucket overlay baseline.
func (m *Manager) R2Branch(ctx context.Context, project, childVersion, parentVersion, bucketName string) error {
	if _, err := exec.LookPath("celld"); err != nil {
		return fmt.Errorf("celld not installed")
	}
	parentBucket := m.versionBucket(project, parentVersion)
	childBucket := m.versionBucket(project, childVersion)
	cmd := exec.CommandContext(ctx, "celld",
		"r2", "branch",
		"--name", bucketName,
		"--parent-bucket", parentBucket,
		"--bucket", childBucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	cmd.Env = append(os.Environ(), m.branchEnv(project, childVersion)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("celld r2 branch: %w: %s", err, string(out))
	}
	return nil
}
