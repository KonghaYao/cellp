package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTailFileFromEndStartsAtEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "celld.log")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tail := TailFileFromEnd(path, 0)
	if got := tail.InitialOffset(); got != 18 {
		t.Fatalf("InitialOffset = %d, want 18 (EOF)", got)
	}
	chunk, newOff, err := tail.ReadSince(tail.InitialOffset())
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk) != 0 {
		t.Fatalf("expected no initial data at EOF, got %q", chunk)
	}
	if newOff != 18 {
		t.Fatalf("newOff = %d, want 18", newOff)
	}
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chunk, newOff, err = tail.ReadSince(tail.InitialOffset())
	if err != nil {
		t.Fatal(err)
	}
	if string(chunk) != "line4\n" {
		t.Fatalf("chunk = %q, want line4 only", chunk)
	}
	if newOff != 24 {
		t.Fatalf("newOff = %d, want 24", newOff)
	}
}

func TestTailFileFromEndMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "celld.log")
	content := "0123456789abcdef"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tail := TailFileFromEnd(path, 8)
	if got := tail.InitialOffset(); got != 8 {
		t.Fatalf("InitialOffset = %d, want 8", got)
	}
	chunk, _, err := tail.ReadSince(tail.InitialOffset())
	if err != nil {
		t.Fatal(err)
	}
	if string(chunk) != "89abcdef" {
		t.Fatalf("chunk = %q", chunk)
	}
}
