package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (m *Manager) appendFleet(args []string, project, version string, jsonOut bool) []string {
	args = append(args,
		"--bucket", m.versionBucket(project, version),
		"--endpoint", m.endpoint,
		"--region", m.region,
	)
	if jsonOut {
		args = append(args, "--json")
	}
	return args
}

func (m *Manager) execCelld(ctx context.Context, project, version string, args []string) ([]byte, error) {
	if _, err := exec.LookPath("celld"); err != nil {
		return nil, ErrCelldUnavailable
	}
	cmd := exec.CommandContext(ctx, "celld", args...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", celldErrPrefix(args), msg)
	}
	return stdout.Bytes(), nil
}

func celldErrPrefix(args []string) string {
	switch {
	case len(args) >= 2:
		return fmt.Sprintf("celld %s %s", args[0], args[1])
	case len(args) == 1:
		return "celld " + args[0]
	default:
		return "celld"
	}
}
