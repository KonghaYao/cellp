package devlocal

import (
	"fmt"
	"os"
	"strings"
)

// StoreBackend is object storage for cellp dev: embedded filesystem S3 or external RustFS.
type StoreBackend string

const (
	StoreLocal   StoreBackend = "local"
	StoreRustFS  StoreBackend = "rustfs"
	StoreFilesystem = "filesystem" // alias for local
)

// ParseStore reads --store / CELLP_STORE (local|filesystem|rustfs).
func ParseStore(flag string) (StoreBackend, error) {
	raw := strings.TrimSpace(flag)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("CELLP_STORE"))
	}
	if raw == "" {
		return StoreLocal, nil
	}
	switch strings.ToLower(raw) {
	case "local", "filesystem", "fs":
		return StoreLocal, nil
	case "rustfs", "s3":
		return StoreRustFS, nil
	default:
		return "", fmt.Errorf("unknown store %q (use local or rustfs)", raw)
	}
}
