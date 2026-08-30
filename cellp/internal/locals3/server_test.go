package locals3_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cellp/cellp/internal/locals3"
)

func TestPutGet(t *testing.T) {
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
	ctx := context.Background()
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("cellp-artifacts"),
		Key:    aws.String("hello.txt"),
		Body:   bytes.NewReader([]byte("hi")),
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("cellp-artifacts"),
		Key:    aws.String("hello.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	got, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Fatalf("got %q", got)
	}
}

// celld diagnose is the storage gate for cellp dev. Skip when celld is not on PATH.
func TestCelldDiagnose(t *testing.T) {
	if _, err := exec.LookPath("celld"); err != nil {
		t.Skip("celld not on PATH")
	}
	dir := t.TempDir()
	srv, err := locals3.Start("127.0.0.1:0", filepath.Join(dir, "s3.bolt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	cmd := exec.Command("celld", "diagnose",
		"--bucket", "s3://cellp-celld/diagnose-probe",
		"--endpoint", srv.Addr,
		"--region", "us-east-1",
	)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=cellpdev",
		"AWS_SECRET_ACCESS_KEY=cellpdev",
		"AWS_REGION=us-east-1",
	)
	out, err := cmd.CombinedOutput()
	t.Logf("celld diagnose:\n%s", out)
	if err != nil {
		t.Fatalf("celld diagnose: %v", err)
	}
}
