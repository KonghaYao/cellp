package agentrun

import (
	"context"
	"errors"
	"strings"
)

// RunExitOutcome maps agentrun.Run errors to process exit code and a log-safe summary line.
func RunExitOutcome(_ context.Context, err error) (exitCode int, logLine string) {
	if err == nil {
		return 0, ""
	}
	if isOnlyContextCancellation(err) {
		return 0, ""
	}
	return 1, SafeRunErrorSummary(err)
}

func isOnlyContextCancellation(err error) bool {
	for _, e := range flattenErrors(err) {
		if e == nil {
			continue
		}
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			continue
		}
		return false
	}
	return true
}

func flattenErrors(err error) []error {
	if err == nil {
		return nil
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		if inner := u.Unwrap(); len(inner) > 0 {
			var out []error
			for _, e := range inner {
				out = append(out, flattenErrors(e)...)
			}
			return out
		}
	}
	if u, ok := err.(interface{ Unwrap() error }); ok {
		if inner := u.Unwrap(); inner != nil {
			return flattenErrors(inner)
		}
	}
	return []error{err}
}

// SafeRunErrorSummary returns a log line without echoing secret material from wrapped errors.
func SafeRunErrorSummary(err error) string {
	var parts []string
	for _, e := range flattenErrors(err) {
		if e == nil || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			continue
		}
		parts = append(parts, sanitizeLogFragment(e.Error()))
	}
	if len(parts) == 0 {
		return "agent run failed"
	}
	return strings.Join(parts, "; ")
}

func sanitizeLogFragment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent run failed"
	}
	// Fail-closed: do not log key=value env-like payloads that may contain secrets.
	if strings.Contains(s, "=") && !strings.HasPrefix(s, "agent ") && !strings.HasPrefix(s, "nodereg ") {
		return "agent run failed"
	}
	return s
}
