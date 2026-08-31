package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cellp/cellp/internal/locals3"
)

func TestServerArtifactURI(t *testing.T) {
	got := ServerArtifactURI("cellp-artifacts", "demo", "v1")
	want := "s3://cellp-artifacts/demo/v1/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestValidateArtifactURI(t *testing.T) {
	bucket := "cellp-artifacts"
	cases := []struct {
		uri    string
		wantOK bool
	}{
		{"s3://cellp-artifacts/demo/v1/", true},
		{"s3://other/demo/v1/", false},
		{"s3://", false},
		{"https://evil.example/bundle", false},
		{"http://169.254.169.254/meta", false},
		{"local/path", true},
	}
	for _, tc := range cases {
		err := validateArtifactURI(tc.uri, bucket)
		if tc.wantOK && err != nil {
			t.Fatalf("%q: %v", tc.uri, err)
		}
		if !tc.wantOK && err == nil {
			t.Fatalf("%q: expected error", tc.uri)
		}
	}
}

func TestFetchLocalPlaceholder(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Bucket: "cellp-artifacts", LocalDir: dir}
	dest := filepath.Join(dir, "out")
	ctx := context.Background()

	got, err := s.Fetch(ctx, "s3://cellp-artifacts/demo/v1/", "", dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("dest dir %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "bundle")); err != nil {
		t.Fatal(err)
	}
}

func TestFetchInvalidDigest(t *testing.T) {
	s := &Store{Bucket: "cellp-artifacts", LocalDir: t.TempDir()}
	_, err := s.Fetch(context.Background(), "s3://cellp-artifacts/x/y/", "sha256:invalid", t.TempDir())
	if err == nil {
		t.Fatal("expected digest error")
	}
}

func TestFetchLocalFile(t *testing.T) {
	dir := t.TempDir()
	name := "mybundle"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("tarbytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Bucket: "cellp-artifacts", LocalDir: dir}
	dest := filepath.Join(dir, "extracted")
	_, err := s.Fetch(context.Background(), "bundles/"+name, "", dest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "bundle.tar"))
	if err != nil || string(data) != "tarbytes" {
		t.Fatalf("bundle.tar: %v %q", err, data)
	}
}

func TestFetchS3FromLocalS3(t *testing.T) {
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
	body := []byte("artifact-payload")
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("cellp-artifacts"),
		Key:    aws.String("demo/v1/bundle.tar"),
		Body:   bytes.NewReader(body),
	})
	if err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(h[:])

	s := &Store{
		Bucket:      "cellp-artifacts",
		LocalDir:    dir,
		S3Endpoint:  srv.Addr,
		S3Region:    "us-east-1",
		S3AccessKey: "cellpdev",
		S3SecretKey: "cellpdev",
	}
	dest := filepath.Join(dir, "fetched")
	_, err = s.Fetch(ctx, "s3://cellp-artifacts/demo/v1/", digest, dest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "bundle.tar"))
	if err != nil || string(got) != string(body) {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestAssertAllowedEndpoint(t *testing.T) {
	if err := assertAllowedEndpoint("http://127.0.0.1:9000"); err != nil {
		t.Fatal(err)
	}
	if err := assertAllowedEndpoint("http://203.0.113.1:9000"); err == nil {
		t.Fatal("expected public endpoint block")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := assertAllowedEndpoint("http://" + ln.Addr().String()); err != nil {
		t.Fatal(err)
	}
}
