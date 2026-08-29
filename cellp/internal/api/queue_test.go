package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/api"
)

func TestQueueMaxDefault(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "")
	if got := api.QueueMax(); got != 10000 {
		t.Fatalf("QueueMax() = %d, want 10000", got)
	}
}

func TestQueueMaxFromEnv(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "500")
	if got := api.QueueMax(); got != 500 {
		t.Fatalf("QueueMax() = %d, want 500", got)
	}
}

func TestPeekLimitValidation(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	for _, limit := range []string{"0", "101"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/queues/tasks/peek?limit="+limit, nil)
		req.Header.Set("Authorization", "Bearer admin")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("limit=%s status=%d body=%s", limit, w.Code, w.Body.String())
		}
		assertAPIError(t, w, "invalid_limit")
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("must not exec celld for invalid limit")
	}
}

func TestPurgeRequiresForce(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		var rdr *bytes.Buffer
		if body == "" {
			rdr = bytes.NewBuffer(nil)
		} else {
			rdr = bytes.NewBufferString(body)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/queues/tasks/purge", rdr)
		req.Header.Set("Authorization", "Bearer admin")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	for _, body := range []string{"", "{}", `{"force":false}`} {
		w := post(body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d resp=%s", body, w.Code, w.Body.String())
		}
		assertAPIError(t, w, "force_required")
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("must not exec celld without force=true")
	}

	w := post(`{"force":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("force true status=%d body=%s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "--force") {
		t.Fatalf("argv missing --force: %s", raw)
	}
}

func TestQueueNotDeclared404(t *testing.T) {
	argsLog := installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/queues/other", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "queue_not_found")
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("must not exec celld for undeclared queue")
	}
}

func TestListQueuesEmptyOk(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{"name":"x"}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/queues", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Queues []any `json:"queues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Queues == nil || len(resp.Queues) != 0 {
		t.Fatalf("queues = %v want []", resp.Queues)
	}
}
