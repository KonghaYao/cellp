package activator

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGlobalPendingBytes     = 256 << 20
	defaultPerVersionPendingBytes = 16 << 20
)

// ConfigFromEnv loads bounded cold-request controls. Empty values preserve the
// implementation defaults, which remain subject to SP-E6 calibration.
func ConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()
	var err error
	if cfg.MaxBufferedBodyBytes, err = envInt64("CELLP_WAKE_MAX_BUFFERED_BODY_BYTES", cfg.MaxBufferedBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.WakeTimeout, err = envDuration("CELLP_WAKE_TIMEOUT", cfg.WakeTimeout); err != nil {
		return Config{}, err
	}
	if cfg.PollInterval, err = envDuration("CELLP_WAKE_POLL_INTERVAL", cfg.PollInterval); err != nil {
		return Config{}, err
	}
	if cfg.RetryAfterSec, err = envInt("CELLP_WAKE_RETRY_AFTER_SECONDS", cfg.RetryAfterSec); err != nil {
		return Config{}, err
	}
	if cfg.GlobalWaitBudget, err = envInt("CELLP_GATEWAY_MAX_PENDING_REQUESTS", cfg.GlobalWaitBudget); err != nil {
		return Config{}, err
	}
	if cfg.PerVersionWaitBudget, err = envInt("CELLP_WAKE_MAX_PENDING_REQUESTS", cfg.PerVersionWaitBudget); err != nil {
		return Config{}, err
	}
	if cfg.GlobalPendingBytes, err = envInt64("CELLP_GATEWAY_MAX_PENDING_BYTES", cfg.GlobalPendingBytes); err != nil {
		return Config{}, err
	}
	if cfg.PerVersionPendingBytes, err = envInt64("CELLP_WAKE_MAX_PENDING_BYTES", cfg.PerVersionPendingBytes); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("activator config: %w", err)
	}
	return cfg, nil
}

// Validate rejects unbounded or internally inconsistent activator controls.
func (c Config) Validate() error {
	if c.MaxBufferedBodyBytes <= 0 || c.WakeTimeout <= 0 || c.PollInterval <= 0 || c.RetryAfterSec <= 0 {
		return fmt.Errorf("timeouts and per-request limits must be positive")
	}
	if c.PollInterval >= c.WakeTimeout {
		return fmt.Errorf("poll interval must be less than wake timeout")
	}
	if c.GlobalWaitBudget <= 0 || c.PerVersionWaitBudget <= 0 || c.PerVersionWaitBudget > c.GlobalWaitBudget {
		return fmt.Errorf("invalid pending request budgets")
	}
	if c.GlobalPendingBytes <= 0 || c.PerVersionPendingBytes <= 0 || c.PerVersionPendingBytes > c.GlobalPendingBytes {
		return fmt.Errorf("invalid pending byte budgets")
	}
	if c.MaxBufferedBodyBytes > c.PerVersionPendingBytes || c.MaxBufferedBodyBytes > c.GlobalPendingBytes {
		return fmt.Errorf("buffered body limit exceeds pending byte budget")
	}
	return nil
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("activator config: %s invalid", key)
	}
	return value, nil
}

func envInt(key string, def int) (int, error) {
	value, err := envInt64(key, int64(def))
	return int(value), err
}

func envInt64(key string, def int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("activator config: %s invalid", key)
	}
	return value, nil
}
