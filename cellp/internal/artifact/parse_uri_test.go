package artifact

import "testing"

func TestParseS3URI(t *testing.T) {
	b, k, ok := parseS3URI("s3://cellp-artifacts/demo/v1/")
	if !ok || b != "cellp-artifacts" || k != "demo/v1" {
		t.Fatalf("got %q %q ok=%v", b, k, ok)
	}
	if _, _, ok := parseS3URI("s3://"); ok {
		t.Fatal("expected false for empty bucket")
	}
	if b, _, ok := parseS3URI("s3://onlybucket"); !ok || b != "onlybucket" {
		t.Fatalf("bucket-only uri ok=%v b=%q", ok, b)
	}
}
