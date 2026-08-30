package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCelldVarsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "celld.vars")
	if err := WriteCelldVarsFile(path, map[string]string{"B": "two", "A": "one"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.HasPrefix(got, "A=one\n") || !strings.Contains(got, "B=two\n") {
		t.Fatalf("got %q", got)
	}
}
