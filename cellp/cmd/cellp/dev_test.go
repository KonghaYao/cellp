package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWranglerName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{
  "name": "my-shop",
  "main": "index.js"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := wranglerName(dir); got != "my-shop" {
		t.Fatalf("got %q", got)
	}
}
