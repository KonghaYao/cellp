package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("payload")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	wrong := "sha256:" + hex.EncodeToString([]byte("other"))
	if err := verifyDigest(dir, wrong); err == nil {
		t.Fatal("expected mismatch")
	}
	right := "sha256:" + hex.EncodeToString(h[:])
	if err := verifyDigest(dir, right); err != nil {
		t.Fatal(err)
	}
}

func TestFetchWrongBucket(t *testing.T) {
	s := &Store{Bucket: "allowed", LocalDir: t.TempDir()}
	_, err := s.Fetch(context.Background(), "s3://other/p/v/", "", t.TempDir())
	if err == nil {
		t.Fatal("expected bucket error")
	}
}
