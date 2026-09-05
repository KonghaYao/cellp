package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PrepareDeployBundle returns a directory to pass to celld deploy. A temp copy is made without
// node_modules (pnpm/npm symlinks break celld file walks). When includeCrons is false,
// triggers.crons are removed from wrangler in that copy; the artifact dir is untouched.
func PrepareDeployBundle(bundleDir string, includeCrons bool) (deployDir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "cellp-deploy-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if err := copyDirFilter(bundleDir, tmp, skipNodeModules); err != nil {
		cleanup()
		return "", nil, err
	}
	if !includeCrons {
		wrPath, raw, err := readWranglerConfigFile(tmp)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		stripped, err := stripCronsFromWranglerJSON(raw)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(wrPath, stripped, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}

func stripCronsFromWranglerJSON(data []byte) ([]byte, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse wrangler: %w", err)
	}
	trRaw, ok := cfg["triggers"]
	if !ok {
		return data, nil
	}
	var triggers map[string]json.RawMessage
	if err := json.Unmarshal(trRaw, &triggers); err != nil {
		return nil, fmt.Errorf("parse triggers: %w", err)
	}
	delete(triggers, "crons")
	if len(triggers) == 0 {
		delete(cfg, "triggers")
	} else {
		merged, err := json.Marshal(triggers)
		if err != nil {
			return nil, err
		}
		cfg["triggers"] = merged
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	return out, nil
}

func skipNodeModules(rel string, _ os.FileInfo) bool {
	if rel == "." {
		return false
	}
	for _, p := range strings.Split(filepath.ToSlash(rel), "/") {
		if p == "node_modules" {
			return true
		}
	}
	return false
}

func copyDir(src, dst string) error {
	return copyDirFilter(src, dst, nil)
}

func copyDirFilter(src, dst string, skip func(rel string, info os.FileInfo) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skip != nil && skip(rel, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
