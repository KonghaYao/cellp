package runtime

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
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

// execCelldOnFleet runs a node-dependent operator command. These commands reach a
// version's namespace through a live celld of that version, so a cold version is woken
// first (pinned against scale-to-zero while the command runs) and the wake is released
// afterwards.
//
// Retrying is deliberately narrow. A mutation may only be re-sent when celld answered
// before dispatching the work — it had no node to run it on — which is the one failure
// that proves nothing executed; a lost response after dispatch would double-apply a put
// or an enqueue. Reads are idempotent, so they also retry when the request was sent and
// the response was lost.
func (m *Manager) execCelldOnFleet(ctx context.Context, project, version string, args []string, idempotent bool) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < operatorFleetAttempts; attempt++ {
		release, err := m.ensureServingNode(ctx, project, version)
		if err != nil {
			return nil, err
		}
		out, execErr := m.execCelld(ctx, project, version, args)
		if release != nil {
			release()
		}
		if execErr == nil {
			return out, nil
		}
		lastErr = execErr
		if !fleetPreDispatch(execErr) && !(idempotent && fleetTransportLost(execErr)) {
			return nil, execErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.fleetRetryDelay()):
		}
	}
	return nil, lastErr
}

const (
	operatorFleetAttempts = 4
	operatorFleetRetryGap = 2 * time.Second
)

func (m *Manager) fleetRetryDelay() time.Duration {
	if m.fleetRetryGap > 0 {
		return m.fleetRetryGap
	}
	return operatorFleetRetryGap
}

// fleetPreDispatch reports celld's refusal to start the work at all: the version has no
// node lease, or no listed lease was usable. Nothing ran, so any command may be retried.
func fleetPreDispatch(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "needs a running fleet") ||
		strings.Contains(msg, "no node in the fleet is reachable")
}

// fleetTransportLost reports a request that may have reached a node without returning an
// answer. Only idempotent commands may be re-sent after this.
func fleetTransportLost(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "reach the namespace at") || strings.Contains(msg, "error sending request")
}

func (m *Manager) execCelld(ctx context.Context, project, version string, args []string) ([]byte, error) {
	cmd, err := celldCommand(ctx, args...)
	if err != nil {
		return nil, ErrCelldUnavailable
	}
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
