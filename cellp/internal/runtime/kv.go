package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrKVNamespaceNotFound is returned when wrangler has no matching kv_namespaces[].id.
var ErrKVNamespaceNotFound = errors.New("kv_namespace_not_found")

// ErrKVKeyNotFound is returned when celld kv get reports no key.
var ErrKVKeyNotFound = errors.New("key_not_found")

// ErrTTLTooSmall is returned when ttl is set and below celld's 60s minimum.
var ErrTTLTooSmall = errors.New("ttl_too_small")

// ErrMetadataTooLarge is returned when --metadata JSON exceeds 1024 bytes.
var ErrMetadataTooLarge = errors.New("metadata_too_large")

// KvKey is one NDJSON row from `celld kv list --json`.
type KvKey struct {
	Name       string          `json:"name"`
	Expiration *int64          `json:"expiration,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

// KvListResult is the parsed listing plus an optional continuation cursor.
type KvListResult struct {
	Keys   []KvKey `json:"keys"`
	Cursor string  `json:"cursor,omitempty"`
}

// KvInfo is `celld kv info --json` totals.
type KvInfo struct {
	Keys   int64 `json:"keys"`
	Bytes  int64 `json:"bytes"`
	Stored int64 `json:"stored"`
}

// KvPutInput is the value and optional flags for `celld kv put`.
type KvPutInput struct {
	Value    []byte
	TTL      *int64
	Metadata string
	Binary   bool
}

// HasKVNamespace reports whether wrangler declares kv_namespaces[].id == ns (verbatim).
func HasKVNamespace(projectDir, ns string) bool {
	b, err := ParseBindings(projectDir)
	if err != nil {
		return false
	}
	for _, k := range b.KV {
		if k.ID == ns {
			return true
		}
	}
	return false
}

// KvList runs `celld kv list`.
func (m *Manager) KvList(ctx context.Context, project, version, projectDir, ns, prefix, cursor string, limit int) (*KvListResult, error) {
	if !HasKVNamespace(projectDir, ns) {
		return nil, ErrKVNamespaceNotFound
	}
	effective := limit
	if effective <= 0 {
		effective = 1000
	}
	if effective > 1000 {
		effective = 1000
	}

	args := []string{"kv", "list", ns}
	if prefix != "" {
		args = append(args, "--prefix", prefix)
	}
	if limit > 0 {
		n := limit
		if n > 1000 {
			n = 1000
		}
		args = append(args, "--limit", strconv.Itoa(n))
	}
	if cursor != "" {
		args = append(args, "--after", cursor)
	}
	args = m.appendFleet(args, project, version, true)

	out, err := m.execCelld(ctx, project, version, args)
	if err != nil {
		return nil, err
	}
	keys, err := parseKVListNDJSON(out)
	if err != nil {
		return nil, err
	}
	result := &KvListResult{Keys: keys}
	if len(keys) == effective && len(keys) > 0 {
		result.Cursor = keys[len(keys)-1].Name
	}
	return result, nil
}

// KvGet runs `celld kv get` and returns stdout as raw bytes (no --json).
func (m *Manager) KvGet(ctx context.Context, project, version, projectDir, ns, key string) ([]byte, error) {
	if !HasKVNamespace(projectDir, ns) {
		return nil, ErrKVNamespaceNotFound
	}
	args := []string{"kv", "get", ns, key}
	args = m.appendFleet(args, project, version, false)
	out, err := m.execCelld(ctx, project, version, args)
	if err != nil {
		if isKVNoKey(err) {
			return nil, ErrKVKeyNotFound
		}
		return nil, err
	}
	return out, nil
}

// KvPut runs `celld kv put` with an inline value or --path for binary/empty/flag-like values.
func (m *Manager) KvPut(ctx context.Context, project, version, projectDir, ns, key string, in KvPutInput) error {
	if !HasKVNamespace(projectDir, ns) {
		return ErrKVNamespaceNotFound
	}
	if in.TTL != nil && *in.TTL < 60 {
		return ErrTTLTooSmall
	}
	if len(in.Metadata) > 1024 {
		return ErrMetadataTooLarge
	}

	args := []string{"kv", "put", ns, key}
	var tmp string
	if kvPutInline(in.Value, in.Binary) {
		args = append(args, string(in.Value))
	} else {
		f, err := os.CreateTemp("", "cellp-kv-put-*")
		if err != nil {
			return err
		}
		tmp = f.Name()
		defer os.Remove(tmp)
		if _, err := f.Write(in.Value); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		args = append(args, "--path", tmp)
	}
	if in.TTL != nil {
		args = append(args, "--ttl", strconv.FormatInt(*in.TTL, 10))
	}
	if in.Metadata != "" {
		args = append(args, "--metadata", in.Metadata)
	}
	args = m.appendFleet(args, project, version, false)
	_, err := m.execCelld(ctx, project, version, args)
	return err
}

// KvDelete runs `celld kv delete` (no --json).
func (m *Manager) KvDelete(ctx context.Context, project, version, projectDir, ns, key string) error {
	if !HasKVNamespace(projectDir, ns) {
		return ErrKVNamespaceNotFound
	}
	args := []string{"kv", "delete", ns, key}
	args = m.appendFleet(args, project, version, false)
	_, err := m.execCelld(ctx, project, version, args)
	return err
}

// KvInfo runs `celld kv info --json`.
func (m *Manager) KvInfo(ctx context.Context, project, version, projectDir, ns string) (*KvInfo, error) {
	if !HasKVNamespace(projectDir, ns) {
		return nil, ErrKVNamespaceNotFound
	}
	args := []string{"kv", "info", ns}
	args = m.appendFleet(args, project, version, true)
	out, err := m.execCelld(ctx, project, version, args)
	if err != nil {
		return nil, err
	}
	var info KvInfo
	if err := json.Unmarshal(bytes.TrimSpace(out), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func kvPutInline(value []byte, binary bool) bool {
	if binary || len(value) == 0 || value[0] == '-' {
		return false
	}
	if bytes.IndexByte(value, 0) >= 0 {
		return false
	}
	return utf8.Valid(value)
}

func parseKVListNDJSON(stdout []byte) ([]KvKey, error) {
	keys := make([]KvKey, 0)
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var key KvKey
		if err := json.Unmarshal([]byte(line), &key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func isKVNoKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no key")
}
