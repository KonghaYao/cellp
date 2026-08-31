package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/runtime"
)

func TestWriteOperatorExecErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
		body string
	}{
		{runtime.ErrCelldUnavailable, http.StatusServiceUnavailable, "celld_unavailable"},
		{runtime.ErrKVKeyNotFound, http.StatusNotFound, "key_not_found"},
		{runtime.ErrTTLTooSmall, http.StatusBadRequest, "ttl_too_small"},
		{runtime.ErrMetadataTooLarge, http.StatusBadRequest, "metadata_too_large"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		writeOperatorExecError(w, tc.err)
		if w.Code != tc.code {
			t.Fatalf("%v: status=%d", tc.err, w.Code)
		}
	}
	w := httptest.NewRecorder()
	writeOperatorExecError(w, errors.New("celld kv get: boom"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("generic celld: %d", w.Code)
	}
}

func TestOperatorError(t *testing.T) {
	e := &operatorError{status: 404, code: "x"}
	if e.Error() != "x" {
		t.Fatal(e.Error())
	}
}
