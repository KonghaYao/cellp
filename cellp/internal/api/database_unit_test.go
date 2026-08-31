package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/runtime"
)

func TestAsInt64(t *testing.T) {
	if n, err := asInt64(float64(42)); err != nil || n != 42 {
		t.Fatalf("float64: %d %v", n, err)
	}
	if n, err := asInt64(int(9)); err != nil || n != 9 {
		t.Fatalf("int: %d %v", n, err)
	}
	if n, err := asInt64(int64(11)); err != nil || n != 11 {
		t.Fatalf("int64: %d %v", n, err)
	}
	if _, err := asInt64("x"); err == nil {
		t.Fatal("expected error")
	}
	if n, err := asInt64(json.Number("7")); err != nil || n != 7 {
		t.Fatalf("json.Number: %d %v", n, err)
	}
}

func TestValidateSQLIdentifier(t *testing.T) {
	if err := validateSQLIdentifier("users"); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLIdentifier("bad!"); err == nil {
		t.Fatal("expected invalid")
	}
	if err := validateSQLIdentifier("_cf_KV"); err == nil {
		t.Fatal("internal table")
	}
}

func TestIsUserVisibleTable(t *testing.T) {
	if !isUserVisibleTable("users") || isUserVisibleTable("sqlite_master") {
		t.Fatal("visibility")
	}
}

func TestEscapeSQL(t *testing.T) {
	if escapeSQLIdentifier(`a"b`) != `a""b` {
		t.Fatal("identifier")
	}
	if escapeSQLStringLiteral(`o'reilly`) != `o''reilly` {
		t.Fatal("literal")
	}
}

func TestWriteDatabaseErrorHTTP(t *testing.T) {
	w := httptest.NewRecorder()
	writeDatabaseError(w, &databaseHTTPError{status: http.StatusNotFound, code: "table_not_found"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("code=%d", w.Code)
	}
	w2 := httptest.NewRecorder()
	writeDatabaseError(w2, fmt.Errorf("plain"))
	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", w2.Code)
	}
}

func TestParseTableOffset(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example/?offset=2", nil)
	if parseTableOffset(r) != 2 {
		t.Fatal("offset")
	}
	r = httptest.NewRequest(http.MethodGet, "http://example/?offset=bad", nil)
	if parseTableOffset(r) != 0 {
		t.Fatal("bad offset")
	}
}

func TestWriteDatabaseExecErrorUnavailable(t *testing.T) {
	w := httptest.NewRecorder()
	writeDatabaseExecError(w, runtime.ErrCelldUnavailable)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestWriteDatabaseExecErrorCelldMessage(t *testing.T) {
	w := httptest.NewRecorder()
	writeDatabaseExecError(w, fmt.Errorf("celld d1 execute: syntax error"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestDatabaseHTTPErrorString(t *testing.T) {
	e := &databaseHTTPError{status: 404, code: "x"}
	if e.Error() != "x" {
		t.Fatal(e.Error())
	}
}

func TestToAPIColumns(t *testing.T) {
	cols := toAPIColumns([]runtime.D1Column{{Name: "id", Type: "INT"}})
	if len(cols) != 1 || cols[0].Name != "id" {
		t.Fatalf("%+v", cols)
	}
}
