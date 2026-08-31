package runtime

import "testing"

func TestParseD1JSONOutput(t *testing.T) {
	stdout := []byte(`{"id":1,"email":"alice@example.com"}
{"id":2,"email":"bob@example.com"}
`)
	stderr := "Executed 1 statement(s) in 12ms\n"
	result, err := parseD1JSONOutput(stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d", len(result.Rows))
	}
	if len(result.Columns) != 2 {
		t.Fatalf("columns = %d", len(result.Columns))
	}
	if result.DurationMS != 12 {
		t.Fatalf("duration_ms = %d", result.DurationMS)
	}
}

func TestParseD1JSONOutputSeconds(t *testing.T) {
	stdout := []byte(`{"cnt":5}`)
	stderr := "Executed 1 statement(s) in 1.50 sec\n"
	result, err := parseD1JSONOutput(stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationMS != 1500 {
		t.Fatalf("duration_ms = %d", result.DurationMS)
	}
}

func TestInferColumnType(t *testing.T) {
	if inferColumnType(float64(3)) != "INTEGER" {
		t.Fatal("expected INTEGER for whole float")
	}
	if inferColumnType(float64(3.14)) != "REAL" {
		t.Fatal("expected REAL")
	}
	if inferColumnType("x") != "TEXT" {
		t.Fatal("expected TEXT")
	}
	if inferColumnType(true) != "INTEGER" {
		t.Fatal("bool")
	}
	if inferColumnType(nil) != "TEXT" {
		t.Fatal("nil")
	}
}

func TestEnsureSQLSemicolon(t *testing.T) {
	if got := ensureSQLSemicolon("SELECT 1"); got != "SELECT 1;" {
		t.Fatalf("got %q", got)
	}
	if got := ensureSQLSemicolon("SELECT 1;"); got != "SELECT 1;" {
		t.Fatalf("got %q", got)
	}
	if got := ensureSQLSemicolon("  SELECT 1  "); got != "SELECT 1;" {
		t.Fatalf("got %q", got)
	}
	if got := ensureSQLSemicolon(""); got != "" {
		t.Fatalf("got %q", got)
	}
}
