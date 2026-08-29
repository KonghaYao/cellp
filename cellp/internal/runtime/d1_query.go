package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrCelldUnavailable is returned when celld is not on PATH.
var ErrCelldUnavailable = errors.New("celld unavailable")

// D1Column describes a query result column.
type D1Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// D1SQLResult is parsed output from `celld d1 execute --json`.
type D1SQLResult struct {
	Columns      []D1Column
	Rows         []map[string]any
	DurationMS   int64
	RowsAffected *int64
}

var executeNoteRE = regexp.MustCompile(`Executed (\d+) statement\(s\) in (?:(\d+)ms|([\d.]+) sec)`)

// ensureSQLSemicolon appends ';' when missing — celld d1 execute requires terminated statements.
func ensureSQLSemicolon(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" || strings.HasSuffix(sql, ";") {
		return sql
	}
	return sql + ";"
}

// D1ExecuteSQL runs arbitrary SQL against a version's D1 database.
func (m *Manager) D1ExecuteSQL(ctx context.Context, project, version, projectDir, sql string) (*D1SQLResult, error) {
	if _, err := exec.LookPath("celld"); err != nil {
		return nil, ErrCelldUnavailable
	}
	database, err := D1DatabaseName(projectDir)
	if err != nil {
		return nil, err
	}
	if database == "" {
		return nil, fmt.Errorf("no d1 database configured")
	}

	sql = ensureSQLSemicolon(sql)
	bucket := m.versionBucket(project, version)
	args := []string{
		"d1", "execute", database,
		"--command", sql,
		projectDir,
		"--bucket", bucket,
		"--endpoint", m.endpoint,
		"--region", m.region,
		"--json",
	}
	cmd := exec.CommandContext(ctx, "celld", args...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("CELLD_VAR_PROJECT_ID=%s", project),
		fmt.Sprintf("CELLD_VAR_VERSION_ID=%s", version),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", m.accessKey),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", m.secretKey),
		fmt.Sprintf("AWS_REGION=%s", m.region),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("celld d1 execute: %s", msg)
	}

	result, err := parseD1JSONOutput(stdout.Bytes(), stderr.String())
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseD1JSONOutput(stdout []byte, stderr string) (*D1SQLResult, error) {
	rows := make([]map[string]any, 0)
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse celld json row: %w", err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := &D1SQLResult{Rows: rows}
	if len(rows) > 0 {
		result.Columns = columnsFromRows(rows)
	}

	if m := executeNoteRE.FindStringSubmatch(stderr); len(m) == 4 {
		switch {
		case m[2] != "":
			ms, _ := strconv.ParseInt(m[2], 10, 64)
			result.DurationMS = ms
		case m[3] != "":
			sec, _ := strconv.ParseFloat(m[3], 64)
			result.DurationMS = int64(math.Round(sec * 1000))
		}
	}

	return result, nil
}

func columnsFromRows(rows []map[string]any) []D1Column {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	cols := make([]D1Column, 0, len(rows[0]))
	for key, val := range rows[0] {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cols = append(cols, D1Column{Name: key, Type: inferColumnType(val)})
	}
	// Stable order for UI: sort by name except preserve sqlite_master column order when obvious.
	if len(cols) > 1 {
		// Keep insertion-ish order from map iteration is random in Go — sort for stability.
		sortColumns(cols)
	}
	return cols
}

func sortColumns(cols []D1Column) {
	for i := 0; i < len(cols); i++ {
		for j := i + 1; j < len(cols); j++ {
			if cols[j].Name < cols[i].Name {
				cols[i], cols[j] = cols[j], cols[i]
			}
		}
	}
}

func inferColumnType(v any) string {
	switch val := v.(type) {
	case float64:
		if math.Mod(val, 1) == 0 {
			return "INTEGER"
		}
		return "REAL"
	case bool:
		return "INTEGER"
	case nil:
		return "TEXT"
	default:
		return "TEXT"
	}
}
