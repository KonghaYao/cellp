package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// WriteCelldVarsFile writes KEY=value lines for CELLD_VARS_FILE.
func WriteCelldVarsFile(path string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := env[k]
		if strings.ContainsAny(v, "\n\x00") {
			return fmt.Errorf("env value for %q contains a newline", k)
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
