package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

type databaseMetadataResp struct {
	DatabaseID      string  `json:"database_id"`
	DatabaseName    string  `json:"database_name"`
	DataBranch      string  `json:"data_branch"`
	ParentVersionID *string `json:"parent_version_id"`
	BranchMethod    string  `json:"branch_method"`
	Status          string  `json:"status"`
}

type databaseTableResp struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	RowCount int64  `json:"row_count"`
}

type databaseTablesResp struct {
	Tables []databaseTableResp `json:"tables"`
}

type databaseColumnResp struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type databaseRowsResp struct {
	Columns []databaseColumnResp `json:"columns"`
	Rows    []map[string]any     `json:"rows"`
	Total   int64                `json:"total"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
}

type databaseQueryReq struct {
	SQL string `json:"sql"`
}

type databaseQueryResp struct {
	Columns      []databaseColumnResp `json:"columns"`
	Rows         []map[string]any     `json:"rows"`
	DurationMS   int64                `json:"duration_ms"`
	RowsAffected *int64               `json:"rows_affected"`
}

type databaseContext struct {
	projectID  string
	versionID  string
	projectDir string
	version    *registry.Version
}

func (s *Server) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveDatabaseContext(r)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}

	dbName, err := runtime.D1DatabaseName(ctx.projectDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if dbName == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "database_not_found"})
		return
	}
	dbID, err := runtime.D1DatabaseID(ctx.projectDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if dbID == "" {
		dbID = dbName
	}

	dataBranch := ctx.version.DataBranch
	if dataBranch == "" {
		dataBranch = ctx.projectID + "/" + ctx.versionID
	}

	writeJSON(w, http.StatusOK, databaseMetadataResp{
		DatabaseID:      dbID,
		DatabaseName:    dbName,
		DataBranch:      dataBranch,
		ParentVersionID: ctx.version.ParentVersionID,
		BranchMethod:    s.databaseBranchMethod(r, ctx),
		Status:          ctx.version.Status,
	})
}

func (s *Server) handleListDatabaseTables(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveDatabaseContext(r)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}

	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir,
		`SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		writeDatabaseExecError(w, err)
		return
	}

	tables := make([]databaseTableResp, 0, len(result.Rows))
	for _, row := range result.Rows {
		name, _ := row["name"].(string)
		typ, _ := row["type"].(string)
		if name == "" {
			continue
		}
		count, err := s.tableRowCount(r, ctx, name)
		if err != nil {
			writeDatabaseExecError(w, err)
			return
		}
		tables = append(tables, databaseTableResp{
			Name:     name,
			Type:     typ,
			RowCount: count,
		})
	}
	writeJSON(w, http.StatusOK, databaseTablesResp{Tables: tables})
}

func (s *Server) handleGetDatabaseTableRows(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveDatabaseContext(r)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}

	tableName := chi.URLParam(r, "tableName")
	if err := validateSQLIdentifier(tableName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_table_name"})
		return
	}
	if err := s.ensureTableExists(r, ctx, tableName); err != nil {
		writeDatabaseError(w, err)
		return
	}

	limit := parseTablePageLimit(r)
	offset := parseTableOffset(r)

	total, err := s.tableRowCount(r, ctx, tableName)
	if err != nil {
		writeDatabaseExecError(w, err)
		return
	}

	columns, err := s.tableColumns(r, ctx, tableName)
	if err != nil {
		writeDatabaseExecError(w, err)
		return
	}

	sql := fmt.Sprintf(`SELECT * FROM "%s" LIMIT %d OFFSET %d`, escapeSQLIdentifier(tableName), limit, offset)
	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, sql)
	if err != nil {
		writeDatabaseExecError(w, err)
		return
	}
	if len(columns) == 0 && len(result.Columns) > 0 {
		columns = toAPIColumns(result.Columns)
	}

	writeJSON(w, http.StatusOK, databaseRowsResp{
		Columns: columns,
		Rows:    result.Rows,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

func (s *Server) handleDatabaseQuery(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveDatabaseContext(r)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}

	var req databaseQueryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sql := strings.TrimSpace(req.SQL)
	if sql == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sql_required"})
		return
	}

	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, sql)
	if err != nil {
		writeDatabaseExecError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, databaseQueryResp{
		Columns:      toAPIColumns(result.Columns),
		Rows:         result.Rows,
		DurationMS:   result.DurationMS,
		RowsAffected: result.RowsAffected,
	})
}

func (s *Server) resolveDatabaseContext(r *http.Request) (*databaseContext, error) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")

	v, err := s.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil {
		return nil, &databaseHTTPError{status: http.StatusInternalServerError, code: err.Error()}
	}
	if v == nil {
		return nil, &databaseHTTPError{status: http.StatusNotFound, code: "version_not_found"}
	}
	if v.Status != registry.StatusReady {
		return nil, &databaseHTTPError{status: http.StatusNotFound, code: "version_not_ready"}
	}

	projectDir := filepath.Join(s.cfg.ArtifactsDir, projectID, versionID)
	dbName, err := runtime.D1DatabaseName(projectDir)
	if err != nil {
		return nil, &databaseHTTPError{status: http.StatusInternalServerError, code: err.Error()}
	}
	if dbName == "" {
		return nil, &databaseHTTPError{status: http.StatusNotFound, code: "database_not_found"}
	}

	return &databaseContext{
		projectID:  projectID,
		versionID:  versionID,
		projectDir: projectDir,
		version:    v,
	}, nil
}

func (s *Server) databaseBranchMethod(r *http.Request, ctx *databaseContext) string {
	if ctx.version.ParentVersionID == nil || *ctx.version.ParentVersionID == "" {
		return "offshoot_export"
	}
	parent, err := s.store.GetVersion(r.Context(), ctx.projectID, *ctx.version.ParentVersionID)
	if err != nil || parent == nil {
		return "offshoot_export"
	}
	plan, err := orch.D1DeployPlanForVersion(ctx.version, parent, ctx.projectDir)
	if err != nil {
		return "offshoot_export"
	}
	if plan.UseBranch {
		return "d1_branch"
	}
	return "offshoot_export"
}

func (s *Server) tableRowCount(r *http.Request, ctx *databaseContext, tableName string) (int64, error) {
	sql := fmt.Sprintf(`SELECT COUNT(*) AS cnt FROM "%s"`, escapeSQLIdentifier(tableName))
	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, sql)
	if err != nil {
		return 0, err
	}
	if len(result.Rows) == 0 {
		return 0, nil
	}
	return asInt64(result.Rows[0]["cnt"])
}

func (s *Server) tableColumns(r *http.Request, ctx *databaseContext, tableName string) ([]databaseColumnResp, error) {
	sql := fmt.Sprintf(`PRAGMA table_info("%s")`, escapeSQLIdentifier(tableName))
	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, sql)
	if err != nil {
		return nil, err
	}
	cols := make([]databaseColumnResp, 0, len(result.Rows))
	for _, row := range result.Rows {
		name, _ := row["name"].(string)
		typ, _ := row["type"].(string)
		if name == "" {
			continue
		}
		if typ == "" {
			typ = "TEXT"
		}
		cols = append(cols, databaseColumnResp{Name: name, Type: typ})
	}
	return cols, nil
}

func (s *Server) ensureTableExists(r *http.Request, ctx *databaseContext, tableName string) error {
	sql := fmt.Sprintf(
		`SELECT 1 FROM sqlite_master WHERE type IN ('table','view') AND name = '%s' LIMIT 1`,
		escapeSQLStringLiteral(tableName),
	)
	result, err := s.runtime.D1ExecuteSQL(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, sql)
	if err != nil {
		return err
	}
	if len(result.Rows) == 0 {
		return &databaseHTTPError{status: http.StatusNotFound, code: "table_not_found"}
	}
	return nil
}

func toAPIColumns(cols []runtime.D1Column) []databaseColumnResp {
	out := make([]databaseColumnResp, len(cols))
	for i, c := range cols {
		out[i] = databaseColumnResp{Name: c.Name, Type: c.Type}
	}
	return out
}

func parseTablePageLimit(r *http.Request) int {
	limit := parsePageLimit(r)
	if limit <= 0 {
		limit = registry.DefaultPageLimit
	}
	if limit > registry.MaxPageLimit {
		limit = registry.MaxPageLimit
	}
	return limit
}

func parseTableOffset(r *http.Request) int {
	v := r.URL.Query().Get("offset")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func validateSQLIdentifier(name string) error {
	if name == "" || len(name) > 128 {
		return fmt.Errorf("invalid identifier")
	}
	for _, r := range name {
		if r == '"' {
			return fmt.Errorf("invalid identifier")
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("invalid identifier")
		}
	}
	return nil
}

func escapeSQLIdentifier(name string) string {
	return strings.ReplaceAll(name, `"`, `""`)
}

func escapeSQLStringLiteral(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case json.Number:
		return n.Int64()
	default:
		return 0, fmt.Errorf("not a number: %T", v)
	}
}

type databaseHTTPError struct {
	status int
	code   string
}

func (e *databaseHTTPError) Error() string { return e.code }

func writeDatabaseError(w http.ResponseWriter, err error) {
	var httpErr *databaseHTTPError
	if errorsAsDatabaseHTTP(err, &httpErr) {
		writeJSON(w, httpErr.status, map[string]string{"error": httpErr.code})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeDatabaseExecError(w http.ResponseWriter, err error) {
	if err == runtime.ErrCelldUnavailable {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "celld_unavailable"})
		return
	}
	msg := strings.TrimPrefix(err.Error(), "celld d1 execute: ")
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func errorsAsDatabaseHTTP(err error, target **databaseHTTPError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*databaseHTTPError); ok {
		*target = e
		return true
	}
	return false
}
