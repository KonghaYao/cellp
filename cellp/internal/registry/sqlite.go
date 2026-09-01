package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const (
	maxRetries = 30
	retryBase  = 50 * time.Millisecond
	retryMax   = 2 * time.Second
)

// SQLiteStore implements Store with WAL mode and busy timeout.
type SQLiteStore struct {
	db             *sql.DB
	ingressPortMin int
	ingressPortMax int
}

// Open opens or creates a SQLite registry database.
func Open(path string) (*SQLiteStore, error) {
	return OpenWithOptions(path, OpenOptions{})
}

// OpenWithOptions opens the registry with optional test-oriented settings.
func OpenWithOptions(path string, opts OpenOptions) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(60000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)&_pragma=mmap_size(268435456)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	minP, maxP := DefaultIngressPortMin, DefaultIngressPortMax
	if opts.IngressPortMin > 0 {
		minP = opts.IngressPortMin
	}
	if opts.IngressPortMax > 0 {
		maxP = opts.IngressPortMax
	}
	if minP > maxP {
		db.Close()
		return nil, fmt.Errorf("ingress port pool invalid: min %d > max %d", minP, maxP)
	}
	s := &SQLiteStore{db: db, ingressPortMin: minP, ingressPortMax: maxP}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	git_remote TEXT,
	prod_version_id TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS versions (
	id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	parent_version_id TEXT,
	git_ref TEXT,
	git_sha TEXT,
	artifact_uri TEXT,
	artifact_digest TEXT,
	data_branch TEXT,
	preview_url TEXT,
	status TEXT NOT NULL,
	error TEXT,
	ttl TEXT,
	env_json TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	ready_at TEXT,
	PRIMARY KEY (project_id, id),
	FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS routes (
	project_id TEXT NOT NULL,
	version_id TEXT NOT NULL,
	active INTEGER NOT NULL DEFAULT 0,
	upstream_host TEXT NOT NULL,
	upstream_port INTEGER NOT NULL,
	PRIMARY KEY (project_id, version_id),
	FOREIGN KEY (project_id, version_id) REFERENCES versions(project_id, id)
);

CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	version_id TEXT NOT NULL,
	step TEXT NOT NULL,
	status TEXT NOT NULL,
	lease_until TEXT,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_versions_status ON versions(project_id, status);
CREATE INDEX IF NOT EXISTS idx_projects_created ON projects(created_at, id);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}
	if err := s.migrateArchiveColumns(); err != nil {
		return err
	}
	if err := s.migrateIngress(); err != nil {
		return err
	}
	if err := s.migratePortAllocations(); err != nil {
		return err
	}
	return s.migrateIngressProjectColumns()
}

func (s *SQLiteStore) migrateIngressProjectColumns() error {
	alters := []string{
		`ALTER TABLE projects ADD COLUMN ingress_tier_b TEXT`,
		`ALTER TABLE projects ADD COLUMN prod_listen_port INTEGER`,
	}
	for _, q := range alters {
		if _, err := s.db.Exec(q); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}

func (s *SQLiteStore) migratePortAllocations() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS port_allocations (
  allocation_id   TEXT PRIMARY KEY,
  port            INTEGER NOT NULL,
  purpose         TEXT NOT NULL CHECK (purpose IN ('ingress_listen', 'celld_upstream')),
  stability       TEXT NOT NULL DEFAULT 'ephemeral'
                  CHECK (stability IN ('ephemeral', 'stable')),
  owner_kind      TEXT NOT NULL
                  CHECK (owner_kind IN ('ingress_binding', 'celld_route')),
  owner_id        TEXT NOT NULL,
  project_id      TEXT,
  gateway_id      TEXT,
  created_at      TEXT NOT NULL,
  released_at     TEXT,
  release_reason  TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_port_alloc_port_active
  ON port_allocations(port) WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_port_alloc_owner_active
  ON port_allocations(owner_kind, owner_id) WHERE released_at IS NULL;
`)
	return err
}

func (s *SQLiteStore) migrateIngress() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS ingress_bindings (
	binding_id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	version_id TEXT,
	role TEXT NOT NULL,
	host TEXT,
	listen_port INTEGER,
	synthetic_host TEXT NOT NULL,
	owner_gateway_id TEXT,
	active INTEGER NOT NULL DEFAULT 0,
	CHECK (role IN ('preview', 'prod')),
	CHECK (host IS NOT NULL OR listen_port IS NOT NULL),
	FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ingress_host_active ON ingress_bindings(host)
	WHERE active = 1 AND host IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ingress_synthetic_active ON ingress_bindings(synthetic_host)
	WHERE active = 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ingress_listen_active ON ingress_bindings(listen_port)
	WHERE active = 1 AND listen_port IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ingress_project_version ON ingress_bindings(project_id, version_id);
`)
	return err
}

func (s *SQLiteStore) migrateArchiveColumns() error {
	alters := []string{
		`ALTER TABLE versions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE versions ADD COLUMN last_access_at TEXT`,
		`ALTER TABLE projects ADD COLUMN previous_prod_version_id TEXT`,
		`ALTER TABLE projects ADD COLUMN previous_prod_at TEXT`,
	}
	for _, q := range alters {
		if _, err := s.db.Exec(q); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}

func withRetry[T any](fn func() (T, error)) (T, error) {
	var zero T
	delay := retryBase
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}
		if !isBusy(err) {
			return zero, err
		}
		lastErr = err
		time.Sleep(delay)
		if delay < retryMax {
			delay *= 2
			if delay > retryMax {
				delay = retryMax
			}
		}
	}
	return zero, fmt.Errorf("sqlite busy after retries: %w", lastErr)
}

func withRetryErr(fn func() error) error {
	_, err := withRetry(func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

func (s *SQLiteStore) CreateProject(ctx context.Context, in CreateProjectInput) (*Project, error) {
	if in.IngressTierB != nil {
		if err := config.ValidateIngressTierBOptional(*in.IngressTierB); err != nil {
			return nil, err
		}
	}
	if in.ProdListenPort != nil {
		minP, maxP := s.ingressPortBounds()
		if err := validateIngressPort(*in.ProdListenPort, minP, maxP); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	return withRetry(func() (*Project, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		_, err = tx.ExecContext(ctx,
			`INSERT INTO projects (id, git_remote, prod_version_id, ingress_tier_b, prod_listen_port, created_at)
			 VALUES (?, ?, NULL, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			in.ID, nullStr(in.GitRemote), nullStr(in.IngressTierB), nullInt(in.ProdListenPort), now.Format(time.RFC3339Nano))
		if err != nil {
			return nil, err
		}

		if in.ProdListenPort != nil {
			pid := in.ID
			_, err := s.reserveStablePortInTx(ctx, tx, ReserveStablePortInput{
				Port:      *in.ProdListenPort,
				OwnerKind: PortOwnerIngressBinding,
				OwnerID:   ProdPortReserveOwnerID(in.ID),
				ProjectID: &pid,
				GatewayID: in.GatewayID,
			})
			if err != nil {
				return nil, err
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, err
		}
		p := &Project{
			ID:             in.ID,
			GitRemote:      in.GitRemote,
			IngressTierB:   in.IngressTierB,
			ProdListenPort: in.ProdListenPort,
			CreatedAt:      now,
		}
		return p, nil
	})
}

func (s *SQLiteStore) GetProject(ctx context.Context, id string) (*Project, error) {
	return withRetry(func() (*Project, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT `+projectSelectCols+` FROM projects WHERE id = ?`, id)
		return scanProject(row)
	})
}

func normalizePageLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageLimit
	}
	if limit > MaxPageLimit {
		return MaxPageLimit
	}
	return limit
}

func (s *SQLiteStore) ListProjects(ctx context.Context, opts ListProjectsOpts) (*ListProjectsPage, error) {
	return withRetry(func() (*ListProjectsPage, error) {
		limit := normalizePageLimit(opts.Limit)
		fetch := limit + 1

		queryFilter := ""
		var queryArgs []any
		if q := strings.TrimSpace(opts.Query); q != "" {
			queryFilter = " AND instr(LOWER(p.id), LOWER(?)) > 0"
			queryArgs = append(queryArgs, q)
		}

		var (
			rows *sql.Rows
			err  error
		)
		if opts.Cursor == "" {
			args := append(queryArgs, fetch)
			rows, err = s.db.QueryContext(ctx, `
				SELECT `+projectListSelectCols+`
				FROM projects p
				WHERE 1=1`+queryFilter+`
				ORDER BY p.created_at ASC, p.id ASC
				LIMIT ?`, args...)
		} else {
			cursorAt, cursorID, err := DecodeCursor(opts.Cursor)
			if err != nil {
				return nil, err
			}
			cursorStr := cursorAt.UTC().Format(time.RFC3339Nano)
			args := append([]any{cursorStr, cursorStr, cursorID}, queryArgs...)
			args = append(args, fetch)
			rows, err = s.db.QueryContext(ctx, `
				SELECT `+projectListSelectCols+`
				FROM projects p
				WHERE (p.created_at > ? OR (p.created_at = ? AND p.id > ?))`+queryFilter+`
				ORDER BY p.created_at ASC, p.id ASC
				LIMIT ?`, args...)
		}
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var items []ProjectListItem
		for rows.Next() {
			p, err := scanProject(rows)
			if err != nil {
				return nil, err
			}
			if p == nil {
				continue
			}
			items = append(items, ProjectListItem{Project: *p})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		page := &ListProjectsPage{Projects: items}
		if len(items) > limit {
			last := items[limit-1]
			page.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
			page.Projects = items[:limit]
		}
		if err := s.fillProjectVersionCounts(ctx, page.Projects); err != nil {
			return nil, err
		}
		return page, nil
	})
}

func (s *SQLiteStore) fillProjectVersionCounts(ctx context.Context, items []ProjectListItem) error {
	if len(items) == 0 {
		return nil
	}
	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	for i, item := range items {
		placeholders[i] = "?"
		args[i] = item.ID
	}
	q := fmt.Sprintf(`
		SELECT project_id, COUNT(*) FROM versions
		WHERE project_id IN (%s)
		GROUP BY project_id`, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	counts := make(map[string]int, len(items))
	for rows.Next() {
		var pid string
		var n int
		if err := rows.Scan(&pid, &n); err != nil {
			return err
		}
		counts[pid] = n
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		items[i].VersionCount = counts[items[i].ID]
	}
	return nil
}

func (s *SQLiteStore) CountVersions(ctx context.Context, projectID string) (int, error) {
	return withRetry(func() (int, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM versions WHERE project_id = ?`, projectID)
		var n int
		err := row.Scan(&n)
		return n, err
	})
}

func (s *SQLiteStore) CreateVersion(ctx context.Context, in CreateVersionInput) (*Version, error) {
	now := time.Now().UTC()
	v := &Version{
		ID:              in.ID,
		ProjectID:       in.ProjectID,
		ParentVersionID: in.ParentVersionID,
		GitRef:          in.GitRef,
		GitSHA:          in.GitSHA,
		ArtifactURI:     in.ArtifactURI,
		ArtifactDigest:  in.ArtifactDigest,
		PreviewURL:      in.PreviewURL,
		Status:          StatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	envJSON, err := marshalEnvJSON(in.Env)
	if err != nil {
		return nil, err
	}
	err = withRetryErr(func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO versions (id, project_id, parent_version_id, git_ref, git_sha, artifact_uri,
				artifact_digest, preview_url, status, env_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.ID, in.ProjectID, nullStr(in.ParentVersionID), in.GitRef, in.GitSHA,
			in.ArtifactURI, in.ArtifactDigest, in.PreviewURL, StatusPending, envJSON,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *SQLiteStore) GetVersion(ctx context.Context, projectID, versionID string) (*Version, error) {
	return withRetry(func() (*Version, error) {
		row := s.db.QueryRowContext(ctx, `
			SELECT id, project_id, parent_version_id, git_ref, git_sha, artifact_uri, artifact_digest,
				data_branch, preview_url, status, error, ttl, created_at, updated_at, ready_at,
				pinned, last_access_at
			FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID)
		return scanVersion(row)
	})
}

func (s *SQLiteStore) ListVersions(ctx context.Context, projectID string, opts ListVersionsOpts) (*ListVersionsPage, error) {
	return withRetry(func() (*ListVersionsPage, error) {
		limit := normalizePageLimit(opts.Limit)
		fetch := limit + 1

		query := `
			SELECT id, project_id, parent_version_id, git_ref, git_sha, artifact_uri, artifact_digest,
				data_branch, preview_url, status, error, ttl, created_at, updated_at, ready_at,
				pinned, last_access_at
			FROM versions
			WHERE project_id = ?`
		args := []interface{}{projectID}

		if opts.Status != "" {
			query += ` AND status = ?`
			args = append(args, opts.Status)
		}
		if opts.Since != nil {
			query += ` AND created_at >= ?`
			args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
		}
		if opts.Cursor != "" {
			cursorAt, cursorID, err := DecodeCursor(opts.Cursor)
			if err != nil {
				return nil, err
			}
			cursorStr := cursorAt.UTC().Format(time.RFC3339Nano)
			query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
			args = append(args, cursorStr, cursorStr, cursorID)
		}
		query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
		args = append(args, fetch)

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var versions []Version
		for rows.Next() {
			v, err := scanVersionRows(rows)
			if err != nil {
				return nil, err
			}
			versions = append(versions, *v)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		page := &ListVersionsPage{Versions: versions}
		if len(versions) > limit {
			last := versions[limit-1]
			page.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
			page.Versions = versions[:limit]
		}
		return page, nil
	})
}

func (s *SQLiteStore) SetVersionPreviewURL(ctx context.Context, projectID, versionID, previewURL string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
			UPDATE versions SET preview_url = ?, updated_at = ?
			WHERE project_id = ? AND id = ?`,
			previewURL, now, projectID, versionID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("version not found")
		}
		return nil
	})
}

func (s *SQLiteStore) UpdateVersionStatus(ctx context.Context, projectID, versionID, status string, errMsg *string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var readyAt interface{}
		var lastAccess interface{}
		if status == StatusReady {
			readyAt = now
			lastAccess = now
		}
		_, err := s.db.ExecContext(ctx, `
			UPDATE versions SET status = ?, error = ?, updated_at = ?,
				ready_at = COALESCE(?, ready_at),
				last_access_at = COALESCE(?, last_access_at, ready_at),
				data_branch = COALESCE(data_branch, ?)
			WHERE project_id = ? AND id = ?`,
			status, nullStr(errMsg), now, readyAt, lastAccess,
			fmt.Sprintf("%s/%s", projectID, versionID),
			projectID, versionID)
		return err
	})
}

func (s *SQLiteStore) CountReadyVersions(ctx context.Context, projectID string) (int, error) {
	return withRetry(func() (int, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM versions WHERE project_id = ? AND status = ?`, projectID, StatusReady)
		var n int
		err := row.Scan(&n)
		return n, err
	})
}

func (s *SQLiteStore) SetRoute(ctx context.Context, route Route) error {
	return withRetryErr(func() error {
		active := 0
		if route.Active {
			active = 1
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO routes (project_id, version_id, active, upstream_host, upstream_port)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(project_id, version_id) DO UPDATE SET
				active = excluded.active,
				upstream_host = excluded.upstream_host,
				upstream_port = excluded.upstream_port`,
			route.ProjectID, route.VersionID, active, route.UpstreamHost, route.UpstreamPort)
		return err
	})
}

func (s *SQLiteStore) SetRouteActive(ctx context.Context, projectID, versionID string, active bool) error {
	return withRetryErr(func() error {
		activeInt := 0
		if active {
			activeInt = 1
		}
		_, err := s.db.ExecContext(ctx,
			`UPDATE routes SET active = ? WHERE project_id = ? AND version_id = ?`,
			activeInt, projectID, versionID)
		return err
	})
}

func (s *SQLiteStore) GetRoute(ctx context.Context, projectID, versionID string) (*Route, error) {
	return withRetry(func() (*Route, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT project_id, version_id, active, upstream_host, upstream_port FROM routes
			 WHERE project_id = ? AND version_id = ?`, projectID, versionID)
		var r Route
		var active int
		if err := row.Scan(&r.ProjectID, &r.VersionID, &active, &r.UpstreamHost, &r.UpstreamPort); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		r.Active = active == 1
		return &r, nil
	})
}

func (s *SQLiteStore) ListActiveRoutes(ctx context.Context, projectID string) ([]Route, error) {
	return withRetry(func() ([]Route, error) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT project_id, version_id, active, upstream_host, upstream_port FROM routes
			 WHERE project_id = ? AND active = 1`, projectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRoutes(rows)
	})
}

func (s *SQLiteStore) ListAllActiveRoutes(ctx context.Context) ([]Route, error) {
	return withRetry(func() ([]Route, error) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT project_id, version_id, active, upstream_host, upstream_port FROM routes
			 WHERE active = 1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRoutes(rows)
	})
}

func scanRoutes(rows *sql.Rows) ([]Route, error) {
	var out []Route
	for rows.Next() {
		var r Route
		var active int
		if err := rows.Scan(&r.ProjectID, &r.VersionID, &active, &r.UpstreamHost, &r.UpstreamPort); err != nil {
			return nil, err
		}
		r.Active = active == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Ping(ctx context.Context) error {
	return withRetryErr(func() error {
		var one int
		return s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one)
	})
}

func (s *SQLiteStore) DeleteRoute(ctx context.Context, projectID, versionID string) error {
	return withRetryErr(func() error {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM routes WHERE project_id = ? AND version_id = ?`, projectID, versionID)
		return err
	})
}

func (s *SQLiteStore) SetProdVersion(ctx context.Context, projectID, versionID string) error {
	return withRetryErr(func() error {
		_, err := s.db.ExecContext(ctx,
			`UPDATE projects SET prod_version_id = ? WHERE id = ?`, versionID, projectID)
		return err
	})
}

func (s *SQLiteStore) SetProdVersionCAS(ctx context.Context, projectID, expected, new string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var prevProd interface{}
		var prevAt interface{}
		if expected != "" {
			prevProd = expected
			prevAt = now
		}
		res, err := s.db.ExecContext(ctx, `
			UPDATE projects SET prod_version_id = ?,
				previous_prod_version_id = CASE WHEN ? = '' THEN previous_prod_version_id ELSE ? END,
				previous_prod_at = CASE WHEN ? = '' THEN previous_prod_at ELSE ? END
			WHERE id = ? AND (prod_version_id = ? OR (prod_version_id IS NULL AND ? = ''))`,
			new, expected, prevProd, expected, prevAt, projectID, nullIfEmpty(expected), expected)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("CAS prod_version failed: expected %q", expected)
		}
		return nil
	})
}

func (s *SQLiteStore) EnqueueJob(ctx context.Context, projectID, versionID, step string) (*Job, error) {
	return withRetry(func() (*Job, error) {
		now := time.Now().UTC()
		j := &Job{
			ID:        uuid.NewString(),
			ProjectID: projectID,
			VersionID: versionID,
			Step:      step,
			Status:    "pending",
			UpdatedAt: now,
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO jobs (id, project_id, version_id, step, status, lease_until, updated_at)
			VALUES (?, ?, ?, ?, ?, NULL, ?)`,
			j.ID, projectID, versionID, step, j.Status, now.Format(time.RFC3339Nano))
		if err != nil {
			return nil, err
		}
		return j, nil
	})
}

func (s *SQLiteStore) CountPendingJobs(ctx context.Context) (int, error) {
	return withRetry(func() (int, error) {
		row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status = 'pending'`)
		var n int
		err := row.Scan(&n)
		return n, err
	})
}

func (s *SQLiteStore) ClaimJob(ctx context.Context, workerID string, lease time.Duration) (*Job, error) {
	_ = workerID
	return withRetry(func() (*Job, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		row := tx.QueryRowContext(ctx, `
			SELECT id, project_id, version_id, step, status, lease_until, updated_at
			FROM jobs
			WHERE status = 'pending'
			   OR (status = 'claimed' AND lease_until < ?)
			ORDER BY updated_at LIMIT 1`, nowStr)
		var j Job
		var leaseUntil sql.NullString
		var updated string
		if err := row.Scan(&j.ID, &j.ProjectID, &j.VersionID, &j.Step, &j.Status, &leaseUntil, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}

		until := now.Add(lease)
		_, err = tx.ExecContext(ctx, `
			UPDATE jobs SET status = 'claimed', lease_until = ?, updated_at = ? WHERE id = ?`,
			until.Format(time.RFC3339Nano), nowStr, j.ID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		j.Status = "claimed"
		j.LeaseUntil = &until
		j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, nowStr)
		return &j, nil
	})
}

func (s *SQLiteStore) CompleteJob(ctx context.Context, jobID string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx,
			`UPDATE jobs SET status = 'completed', lease_until = NULL, updated_at = ? WHERE id = ?`,
			now, jobID)
		return err
	})
}

func (s *SQLiteStore) UpdateJobStep(ctx context.Context, jobID, step string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx,
			`UPDATE jobs SET step = ?, updated_at = ? WHERE id = ?`, step, now, jobID)
		return err
	})
}

func (s *SQLiteStore) FailJob(ctx context.Context, jobID string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx,
			`UPDATE jobs SET status = 'failed', lease_until = NULL, updated_at = ? WHERE id = ?`,
			now, jobID)
		return err
	})
}

func (s *SQLiteStore) PurgeCompletedJobs(ctx context.Context, olderThan time.Time) (int64, error) {
	return withRetry(func() (int64, error) {
		cutoff := olderThan.UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
			DELETE FROM jobs
			WHERE status IN ('completed', 'failed')
			  AND updated_at < ?`, cutoff)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	})
}

func (s *SQLiteStore) PurgeDestroyedVersions(ctx context.Context, olderThan time.Time) (int64, error) {
	return withRetry(func() (int64, error) {
		cutoff := olderThan.UTC().Format(time.RFC3339Nano)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()

		_, err = tx.ExecContext(ctx, `
			DELETE FROM routes
			WHERE active = 0
			  AND EXISTS (
			    SELECT 1 FROM versions v
			    WHERE v.project_id = routes.project_id
			      AND v.id = routes.version_id
			      AND v.status = ?
			      AND v.updated_at < ?
			      AND NOT EXISTS (
			        SELECT 1 FROM projects p
			        WHERE p.id = v.project_id AND p.prod_version_id = v.id
			      )
			  )`, StatusDestroyed, cutoff)
		if err != nil {
			return 0, err
		}

		res, err := tx.ExecContext(ctx, `
			DELETE FROM versions
			WHERE status = ?
			  AND updated_at < ?
			  AND NOT EXISTS (
			    SELECT 1 FROM routes r
			    WHERE r.project_id = versions.project_id
			      AND r.version_id = versions.id
			      AND r.active = 1
			  )
			  AND NOT EXISTS (
			    SELECT 1 FROM projects p
			    WHERE p.id = versions.project_id AND p.prod_version_id = versions.id
			  )`, StatusDestroyed, cutoff)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return n, nil
	})
}

func scanVersion(row *sql.Row) (*Version, error) {
	var v Version
	var parent, artifactURI, digest, branch, preview, status, errMsg, ttl, created, updated, ready, lastAccess sql.NullString
	var pinned int
	if err := row.Scan(&v.ID, &v.ProjectID, &parent, &v.GitRef, &v.GitSHA, &artifactURI, &digest,
		&branch, &preview, &status, &errMsg, &ttl, &created, &updated, &ready, &pinned, &lastAccess); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	fillVersion(&v, parent, artifactURI, digest, branch, preview, status, errMsg, ttl, created, updated, ready, pinned, lastAccess)
	return &v, nil
}

func scanVersionRows(rows *sql.Rows) (*Version, error) {
	var v Version
	var parent, artifactURI, digest, branch, preview, status, errMsg, ttl, created, updated, ready, lastAccess sql.NullString
	var pinned int
	if err := rows.Scan(&v.ID, &v.ProjectID, &parent, &v.GitRef, &v.GitSHA, &artifactURI, &digest,
		&branch, &preview, &status, &errMsg, &ttl, &created, &updated, &ready, &pinned, &lastAccess); err != nil {
		return nil, err
	}
	fillVersion(&v, parent, artifactURI, digest, branch, preview, status, errMsg, ttl, created, updated, ready, pinned, lastAccess)
	return &v, nil
}

func fillVersion(v *Version, parent, artifactURI, digest, branch, preview, status, errMsg, ttl, created, updated, ready sql.NullString, pinned int, lastAccess sql.NullString) {
	if parent.Valid {
		v.ParentVersionID = &parent.String
	}
	if artifactURI.Valid {
		v.ArtifactURI = artifactURI.String
	}
	if digest.Valid {
		v.ArtifactDigest = digest.String
	}
	if branch.Valid {
		v.DataBranch = branch.String
	}
	if preview.Valid {
		v.PreviewURL = preview.String
	}
	if status.Valid {
		v.Status = status.String
	}
	if errMsg.Valid {
		v.Error = &errMsg.String
	}
	if ttl.Valid {
		t, _ := time.Parse(time.RFC3339Nano, ttl.String)
		v.TTL = &t
	}
	if created.Valid {
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	}
	if updated.Valid {
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated.String)
	}
	if ready.Valid {
		t, _ := time.Parse(time.RFC3339Nano, ready.String)
		v.ReadyAt = &t
	}
	v.Pinned = pinned != 0
	if lastAccess.Valid {
		t, _ := time.Parse(time.RFC3339Nano, lastAccess.String)
		v.LastAccessAt = &t
	}
}

func nullStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ExecTestSQL runs arbitrary SQL (tests only).
func (s *SQLiteStore) ExecTestSQL(ctx context.Context, query string, args ...any) error {
	return withRetryErr(func() error {
		_, err := s.db.ExecContext(ctx, query, args...)
		return err
	})
}

func (s *SQLiteStore) SetVersionPinned(ctx context.Context, projectID, versionID string, pinned bool) error {
	return withRetryErr(func() error {
		p := 0
		if pinned {
			p = 1
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx,
			`UPDATE versions SET pinned = ?, updated_at = ? WHERE project_id = ? AND id = ?`,
			p, now, projectID, versionID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("version not found")
		}
		return nil
	})
}

func (s *SQLiteStore) TouchLastAccess(ctx context.Context, projectID, versionID string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx,
			`UPDATE versions SET last_access_at = ? WHERE project_id = ? AND id = ?`,
			now, projectID, versionID)
		return err
	})
}

func (s *SQLiteStore) GetVersionEnv(ctx context.Context, projectID, versionID string) (map[string]string, error) {
	return withRetry(func() (map[string]string, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT env_json FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID)
		var raw sql.NullString
		if err := row.Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("version not found")
			}
			return nil, err
		}
		return unmarshalEnvJSON(raw.String), nil
	})
}

func (s *SQLiteStore) SetVersionEnv(ctx context.Context, projectID, versionID string, env map[string]string) error {
	payload, err := marshalEnvJSON(env)
	if err != nil {
		return err
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx,
			`UPDATE versions SET env_json = ?, updated_at = ? WHERE project_id = ? AND id = ?`,
			payload, now, projectID, versionID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("version not found")
		}
		return nil
	})
}

func marshalEnvJSON(env map[string]string) (string, error) {
	if env == nil {
		env = map[string]string{}
	}
	b, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalEnvJSON(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return map[string]string{}
	}
	return out
}

func (s *SQLiteStore) ListAllReadyVersions(ctx context.Context) ([]Version, error) {
	return withRetry(func() ([]Version, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, project_id, parent_version_id, git_ref, git_sha, artifact_uri, artifact_digest,
				data_branch, preview_url, status, error, ttl, created_at, updated_at, ready_at,
				pinned, last_access_at
			FROM versions WHERE status = ?`, StatusReady)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Version
		for rows.Next() {
			v, err := scanVersionRows(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, *v)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) CountChildVersions(ctx context.Context, projectID, parentVersionID string) (int, error) {
	return withRetry(func() (int, error) {
		row := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM versions
			WHERE project_id = ? AND parent_version_id = ?
			  AND status IN (?, ?)`,
			projectID, parentVersionID, StatusReady, StatusArchived)
		var n int
		err := row.Scan(&n)
		return n, err
	})
}
