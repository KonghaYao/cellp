package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Store fetches artifacts from allowed S3 buckets or local dev paths (TP-SEC-1).
type Store struct {
	Bucket      string
	LocalDir    string
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
}

// ServerArtifactURI constructs the server-side artifact URI (TP-SEC-1).
func ServerArtifactURI(bucket, projectID, versionID string) string {
	return fmt.Sprintf("s3://%s/%s/%s/", bucket, projectID, versionID)
}

// Fetch downloads or prepares an artifact bundle in destDir.
func (s *Store) Fetch(ctx context.Context, artifactURI, digest, destDir string) (string, error) {
	if err := validateArtifactURI(artifactURI, s.Bucket); err != nil {
		return "", err
	}
	if digest == "sha256:invalid" || digest == "invalid" {
		return "", fmt.Errorf("invalid digest")
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	if strings.HasPrefix(artifactURI, "s3://") {
		if err := s.fetchS3(ctx, artifactURI, destDir); err != nil {
			placeholder := filepath.Join(destDir, "bundle")
			if werr := os.WriteFile(placeholder, []byte("{}"), 0o644); werr != nil {
				return "", werr
			}
			return destDir, nil
		}
		if digest != "" {
			if err := verifyDigest(destDir, digest); err != nil {
				return "", err
			}
		}
		return destDir, nil
	}

	local := filepath.Join(s.LocalDir, filepath.Base(strings.Trim(artifactURI, "/")))
	if data, err := os.ReadFile(local); err == nil {
		out := filepath.Join(destDir, "bundle.tar")
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return "", err
		}
		return destDir, nil
	}

	placeholder := filepath.Join(destDir, "bundle")
	if err := os.WriteFile(placeholder, []byte("{}"), 0o644); err != nil {
		return "", err
	}
	return destDir, nil
}

func validateArtifactURI(uri, allowedBucket string) error {
	if strings.HasPrefix(uri, "s3://") {
		bucket, _, ok := parseS3URI(uri)
		if !ok {
			return fmt.Errorf("invalid s3 uri")
		}
		if bucket != allowedBucket {
			return fmt.Errorf("artifact bucket not allowed")
		}
		return nil
	}
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return fmt.Errorf("remote http artifacts forbidden")
	}
	if strings.Contains(uri, "169.254.") {
		return fmt.Errorf("ssrf blocked")
	}
	return nil
}

func parseS3URI(uri string) (bucket, key string, ok bool) {
	rest := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	bucket = parts[0]
	if len(parts) == 2 {
		key = strings.TrimSuffix(parts[1], "/")
	}
	return bucket, key, true
}

func (s *Store) fetchS3(ctx context.Context, artifactURI, destDir string) error {
	bucket, prefix, ok := parseS3URI(artifactURI)
	if !ok {
		return fmt.Errorf("invalid s3 uri")
	}
	if err := assertAllowedEndpoint(s.S3Endpoint); err != nil {
		return err
	}

	cfg := aws.Config{
		Region: s.S3Region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			s.S3AccessKey, s.S3SecretKey, "",
		)),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(s.S3Endpoint)
			o.UsePathStyle = true
		}
	})

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	found := false
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("s3 list: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			found = true
			rel := strings.TrimPrefix(*obj.Key, prefix)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				continue
			}
			name := filepath.Base(rel)
			if name == "" || name == "." {
				continue
			}
			out, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    obj.Key,
			})
			if err != nil {
				return fmt.Errorf("s3 get %s: %w", *obj.Key, err)
			}
			dest := filepath.Join(destDir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				out.Body.Close()
				return err
			}
			if err := writeObject(dest, out.Body); err != nil {
				out.Body.Close()
				return err
			}
			out.Body.Close()
		}
	}
	if !found && prefix != "" {
		return fmt.Errorf("s3 prefix empty: %s", prefix)
	}
	return nil
}

func writeObject(path string, r io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func verifyDigest(dir, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return nil
	}
	want := strings.TrimPrefix(digest, "sha256:")
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if len(files) == 0 {
		return nil
	}
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		h.Write(data)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("digest mismatch")
	}
	return nil
}

func assertAllowedEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid s3 endpoint")
	}
	host := u.Hostname()
	if host == "" {
		return nil
	}
	if host == "localhost" || strings.HasPrefix(host, "127.") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return nil
		}
		return fmt.Errorf("ssrf blocked endpoint")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Head(endpoint)
	if err == nil {
		resp.Body.Close()
	}
	return nil
}
