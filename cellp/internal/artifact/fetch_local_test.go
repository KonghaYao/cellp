package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchLocalTarGzLayout(t *testing.T) {
	dir := t.TempDir()
	name := "bundle.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("gz"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Bucket: "cellp-artifacts", LocalDir: dir}
	dest := filepath.Join(dir, "out")
	_, err := s.Fetch(context.Background(), name, "", dest)
	if err != nil {
		t.Fatal(err)
	}
}
