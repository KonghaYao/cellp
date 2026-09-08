package controllermtls

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WaitForStartup waits until the listener is ready or Run exits before becoming ready.
// ready must return the current readiness channel (Run may replace it when starting).
func WaitForStartup(ctx context.Context, ready func() <-chan struct{}, done <-chan struct{}, runErr <-chan error) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-ready():
			select {
			case <-done:
				if serverErr := drainRunErr(runErr); serverErr != nil {
					return serverErr
				}
				return errors.New("controller mTLS listener stopped during startup")
			default:
			}
			select {
			case serverErr := <-runErr:
				if serverErr == nil {
					serverErr = errors.New("controller mTLS listener stopped during startup")
				}
				return fmt.Errorf("remote control listener: %w", serverErr)
			default:
				return nil
			}
		case serverErr := <-runErr:
			if serverErr == nil {
				serverErr = errors.New("controller mTLS listener stopped during startup")
			}
			return fmt.Errorf("remote control listener: %w", serverErr)
		case <-done:
			if serverErr := drainRunErr(runErr); serverErr != nil {
				return serverErr
			}
			return errors.New("controller mTLS listener stopped during startup")
		case <-ctx.Done():
			return ctx.Err()
		default:
			if time.Now().After(deadline) {
				return errors.New("controller mTLS listener startup timed out")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func drainRunErr(runErr <-chan error) error {
	select {
	case serverErr := <-runErr:
		if serverErr == nil {
			serverErr = errors.New("controller mTLS listener stopped during startup")
		}
		return fmt.Errorf("remote control listener: %w", serverErr)
	default:
		return nil
	}
}
