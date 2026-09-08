package serve

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

const registryBusyPollInterval = 20 * time.Millisecond

// registryCallEventually retries registry observation calls until deadline.
// Busy/locked classification uses registry.IsSQLiteBusy (single implementation in registry package).
func registryCallEventually(t *testing.T, deadline time.Time, op func() error) error {
	t.Helper()
	var lastErr error
	for time.Now().Before(deadline) {
		err := op()
		if err == nil {
			return nil
		}
		if !registry.IsSQLiteBusy(err) {
			return err
		}
		lastErr = err
		time.Sleep(registryBusyPollInterval)
	}
	if lastErr != nil {
		return fmt.Errorf("sqlite busy until deadline: %w", lastErr)
	}
	return errors.New("deadline exceeded before registry operation succeeded")
}
