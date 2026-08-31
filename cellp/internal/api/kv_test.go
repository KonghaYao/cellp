package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func installOperatorFakeCelld(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(root, "celld-args.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + argsLog + "\n" +
		"case \"$1\" in\n" +
		"kv)\n" +
		"  case \"$2\" in\n" +
		"  get)\n" +
		"    key=\"$4\"\n" +
		"    if [ \"$key\" = \"nul\" ]; then printf 'a\\0b'; else printf 'hello'; fi\n" +
		"    ;;\n" +
		"  list) printf '%s\\n' '{\"name\":\"k0\"}' ;;\n" +
		"  info) printf '%s\\n' '{\"keys\":1,\"bytes\":5,\"stored\":1}' ;;\n" +
		"  delete) exit 0 ;;\n" +
		"  put) exit 0 ;;\n" +
		"  esac\n" +
		"  ;;\n" +
		"queue) printf '%s\\n' '{\"ok\":true}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return argsLog
}

func setupKVQueueVersion(t *testing.T, store *registry.SQLiteStore, artifactsDir string) {
	t.Helper()
	setupReadyVersion(t, store, artifactsDir, `{
  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }],
  "queues": {
    "producers": [{ "binding": "TASKS", "queue": "tasks" }],
    "consumers": [{ "queue": "events", "dead_letter_queue": "events-dlq" }]
  }
}`)
}

func TestListKVNamespaces(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Namespaces []struct {
			ID      string `json:"id"`
			Binding string `json:"binding"`
		} `json:"namespaces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Namespaces) != 1 || resp.Namespaces[0].ID != "ns-1" || resp.Namespaces[0].Binding != "KV" {
		t.Fatalf("namespaces = %+v", resp.Namespaces)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		raw, _ := os.ReadFile(argsLog)
		if strings.Contains(string(raw), "kv") {
			t.Fatalf("list namespaces must not invoke celld kv: %s", raw)
		}
	}
}

func TestGetKVValueEncoding(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	get := func(key string) map[string]string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv/ns-1/keys/"+key, nil)
		req.Header.Set("Authorization", "Bearer admin")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	utf := get("plain")
	if utf["encoding"] != "utf-8" || utf["value"] != "hello" || utf["key"] != "plain" {
		t.Fatalf("utf-8 resp = %+v", utf)
	}

	bin := get("nul")
	if bin["encoding"] != "base64" {
		t.Fatalf("nul encoding = %q", bin["encoding"])
	}
	raw, err := base64.StdEncoding.DecodeString(bin["value"])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte{'a', 0, 'b'}) {
		t.Fatalf("decoded = %q", raw)
	}
}

func TestPutKVValue(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	body := bytes.NewBufferString(`{"value":"hello"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "put") {
		t.Fatalf("expected put in argv log: %s", raw)
	}
}

func TestUnknownKVNamespace404(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv/ns-nope", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "kv_namespace_not_found")
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("must not exec celld for unknown ns")
	}
}

func TestKVVersionNotReady404(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "version_not_ready")
}

func TestListKVKeysAndInfo(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv/ns-1/keys?limit=10", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list keys status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv/ns-1", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("info status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteKVValue(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/demo/versions/v1/kv/ns-1/keys/mykey", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "delete") {
		t.Fatalf("argv: %s", raw)
	}
}

func TestPutKVValueWithTTL(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	body := bytes.NewBufferString(`{"value":"x","ttl":120}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPutKVValueTTLTooSmall(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	body := bytes.NewBufferString(`{"value":"x","ttl":30}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "ttl_too_small")
}
