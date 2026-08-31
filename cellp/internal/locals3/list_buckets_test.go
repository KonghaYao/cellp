package locals3_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cellp/cellp/internal/locals3"
)

func TestListBuckets(t *testing.T) {
	dir := t.TempDir()
	srv, err := locals3.Start("127.0.0.1:0", filepath.Join(dir, "s3.bolt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			"cellpdev", "cellpdev", "",
		)),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(srv.Addr)
		o.UsePathStyle = true
	})
	out, err := client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Buckets) < 1 {
		t.Fatalf("buckets=%v", out.Buckets)
	}
}
