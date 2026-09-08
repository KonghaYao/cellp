package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// celldBinary resolves the celld executable. CELLP_CELLD_BIN overrides PATH lookup
// (dev lab builds, CI, or when ~/.local/bin/celld is blocked by the host).
func celldBinary() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CELLP_CELLD_BIN")); v != "" {
		st, err := os.Stat(v)
		if err != nil {
			return "", fmt.Errorf("CELLP_CELLD_BIN: %w", err)
		}
		if st.IsDir() {
			return "", fmt.Errorf("CELLP_CELLD_BIN is a directory: %s", v)
		}
		return v, nil
	}
	return exec.LookPath("celld")
}

func celldCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	bin, err := celldBinary()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, bin, args...), nil
}
