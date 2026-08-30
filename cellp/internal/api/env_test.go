package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestGetPutWorkerEnv(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	body := bytes.NewBufferString(`{"vars":{"GREETING":"hello","PROJECT_ID":"evil","CELLP_X":"no"}}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/env", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/env", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Vars []struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Source string `json:"source"`
		} `json:"vars"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]string{}
	src := map[string]string{}
	for _, v := range got.Vars {
		byKey[v.Key] = v.Value
		src[v.Key] = v.Source
	}
	if byKey["GREETING"] != "hello" || src["GREETING"] != "override" {
		t.Fatalf("GREETING = %#v src=%#v", byKey, src)
	}
	if byKey["PROJECT_ID"] != "demo" || src["PROJECT_ID"] != "platform" {
		t.Fatalf("PROJECT_ID = %#v src=%#v", byKey["PROJECT_ID"], src["PROJECT_ID"])
	}
}

func TestPutWorkerEnvInvalidKey(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	body := bytes.NewBufferString(`{"vars":{"bad-key":"x"}}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/env", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
