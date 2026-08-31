package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetR2BucketNamesAndQueues(t *testing.T) {
	dir := t.TempDir()
	wrangler := `{
  "r2_buckets": [{"binding": "BUCKET", "bucket_name": "old"}],
  "queues": {
    "producers": [{"binding": "Q", "queue": "old-q"}],
    "consumers": [{"queue": "old-q"}]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setR2BucketNames(dir, []R2Bucket{{Binding: "BUCKET", BucketName: "new-bucket"}}); err != nil {
		t.Fatal(err)
	}
	if err := setQueueNames(dir, []QueueBinding{{Binding: "Q", Name: "new-q"}}); err != nil {
		t.Fatal(err)
	}
	got, err := ParseBindings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.R2) != 1 || got.R2[0].BucketName != "new-bucket" {
		t.Fatalf("r2 %+v", got.R2)
	}
}
