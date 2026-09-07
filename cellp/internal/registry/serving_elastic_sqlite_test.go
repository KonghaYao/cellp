package registry

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestMigrationIdempotentEndpointsTable(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/registry.sqlite"
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var tbl int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'runtime_replica_endpoints'`).Scan(&tbl); err != nil || tbl != 1 {
		t.Fatalf("endpoints table: n=%d err=%v", tbl, err)
	}
	var assignedCol int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('runtime_replicas') WHERE name = 'assigned_node_generation'`).Scan(&assignedCol); err != nil || assignedCol != 1 {
		t.Fatalf("assigned_node_generation column: n=%d err=%v", assignedCol, err)
	}
	for _, column := range []string{"lease_expires_at", "attempt_token"} {
		var count int
		if err := s2.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('runtime_agent_commands') WHERE name = ?`, column).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s column after reopen: n=%d err=%v", column, count, err)
		}
	}
}

func TestMigrationFromHistoricalElasticDDL(t *testing.T) {
	path := t.TempDir() + "/historical.sqlite"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	historical := `
CREATE TABLE projects (id TEXT PRIMARY KEY, git_remote TEXT, prod_version_id TEXT, created_at TEXT NOT NULL);
CREATE TABLE versions (id TEXT NOT NULL, project_id TEXT NOT NULL, parent_version_id TEXT, git_ref TEXT, git_sha TEXT, artifact_uri TEXT, artifact_digest TEXT, data_branch TEXT, preview_url TEXT, status TEXT NOT NULL, error TEXT, ttl TEXT, env_json TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, ready_at TEXT, PRIMARY KEY(project_id,id));
CREATE TABLE routes (project_id TEXT NOT NULL, version_id TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 0, upstream_host TEXT NOT NULL, upstream_port INTEGER NOT NULL, PRIMARY KEY(project_id,version_id));
CREATE TABLE jobs (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, version_id TEXT NOT NULL, step TEXT NOT NULL, status TEXT NOT NULL, lease_until TEXT, updated_at TEXT NOT NULL);
CREATE TABLE runtime_nodes (node_id TEXT PRIMARY KEY, capacity_units INTEGER NOT NULL DEFAULT 0, cordoned INTEGER NOT NULL DEFAULT 0, lease_expiry TEXT NOT NULL, generation INTEGER NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE runtime_replicas (replica_id TEXT PRIMARY KEY, project_id TEXT NOT NULL, version_id TEXT NOT NULL, node_id TEXT NOT NULL, generation INTEGER NOT NULL, state TEXT NOT NULL, valid_until TEXT, updated_at TEXT NOT NULL);
CREATE TABLE runtime_agent_commands (idempotency_key TEXT PRIMARY KEY, action TEXT NOT NULL, node_id TEXT NOT NULL, project_id TEXT NOT NULL, version_id TEXT NOT NULL, replica_id TEXT NOT NULL, generation INTEGER NOT NULL, status TEXT NOT NULL, result_state TEXT, reason TEXT, expires_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO projects VALUES ('demo',NULL,NULL,'2025-01-01T00:00:00Z');
INSERT INTO versions VALUES ('v1','demo',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'ready',NULL,NULL,NULL,'2025-01-01T00:00:00Z','2025-01-01T00:00:00Z',NULL);
INSERT INTO runtime_nodes VALUES ('n1',1,0,'2099-01-01T00:00:00Z',1,'2025-01-01T00:00:00Z');
INSERT INTO runtime_replicas VALUES ('old-r','demo','v1','n1',1,'ready','2099-01-01T00:00:00Z','2025-01-01T00:00:00Z');
INSERT INTO runtime_agent_commands VALUES ('running','start_replica','n1','demo','v1','old-r',1,'running',NULL,NULL,'2099-01-01T00:00:00Z','2025-01-01T00:00:00Z');
INSERT INTO runtime_agent_commands VALUES ('terminal','start_replica','n1','demo','v1','old-r',1,'succeeded','ready',NULL,'2099-01-01T00:00:00Z','2025-01-01T00:00:00Z');`
	if _, err := db.Exec(historical); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWithOptions(path, OpenOptions{AgentCommandLease: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"assigned_node_generation", "lease_expires_at", "attempt_token"} {
		var n int
		query := `SELECT COUNT(*) FROM pragma_table_info('runtime_agent_commands') WHERE name = ?`
		if column == "assigned_node_generation" {
			query = `SELECT COUNT(*) FROM pragma_table_info('runtime_replicas') WHERE name = ?`
		}
		if err := store.db.QueryRow(query, column).Scan(&n); err != nil || n != 1 {
			t.Fatalf("column %s: n=%d err=%v", column, n, err)
		}
	}
	if rep, err := store.ValidateAgentAssignment(context.Background(), contract.CommandScope{ReplicaID: "old-r"}, time.Now()); err == nil || rep != nil {
		t.Fatalf("legacy NULL node generation entered assignment: rep=%+v err=%v", rep, err)
	}
	if _, err := store.db.Exec(`UPDATE versions SET elastic_enrolled = 1 WHERE project_id = 'demo' AND id = 'v1';
INSERT INTO runtime_replica_endpoints (replica_id, listen_host, listen_port, endpoint_state, valid_until, updated_at)
VALUES ('old-r','127.0.0.1',9901,'ready','2099-01-01T00:00:00Z','2025-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(context.Background())
	if err != nil || len(snap.EndpointSets) != 0 {
		t.Fatalf("legacy NULL node generation entered snapshot: %+v err=%v", snap.EndpointSets, err)
	}
	terminal, err := store.ClaimAgentCommand(context.Background(), AgentCommand{IdempotencyKey: "terminal", Action: contract.ActionStartReplica, NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "old-r", Generation: 1})
	if err != nil || terminal.Claimed || terminal.Command.Status != "succeeded" {
		t.Fatalf("terminal replay: %+v err=%v", terminal, err)
	}
	time.Sleep(2 * time.Millisecond)
	takeover, err := store.ClaimAgentCommand(context.Background(), AgentCommand{IdempotencyKey: "running", Action: contract.ActionStartReplica, NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "old-r", Generation: 1})
	if err != nil || !takeover.Claimed || takeover.Command.AttemptToken == "" {
		t.Fatalf("legacy running takeover: %+v err=%v", takeover, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
}

func TestValidateAssignmentSeparatesDesireAndNodeGeneration(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/generation.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateProject(ctx, CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{NodeID: "n1", CapacityUnits: 1, Generation: 7, LeaseExpiry: exp}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, AssignmentClaim{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 7, ValidUntil: exp}); err != nil {
		t.Fatal(err)
	}
	scope := contract.CommandScope{NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "r1", Generation: 1, LeaseExpiry: exp}
	if _, err := store.ValidateAgentAssignment(ctx, scope, time.Now().UTC()); err != nil {
		t.Fatalf("node generation may differ from desire generation: %v", err)
	}
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{NodeID: "n1", CapacityUnits: 1, Generation: 8, LeaseExpiry: exp}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateAgentAssignment(ctx, scope, time.Now().UTC()); !errors.Is(err, ErrObservationStale) {
		t.Fatalf("stale assigned node generation accepted: %v", err)
	}
}

func TestAgentCommandStaleLeaseTakeoverIsAttemptFenced(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/command-lease.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	command := AgentCommand{IdempotencyKey: "key", Action: contract.ActionStartReplica, NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "r1", Generation: 1}
	first, err := store.ClaimAgentCommand(ctx, command)
	if err != nil || !first.Claimed || first.Command.AttemptToken == "" {
		t.Fatalf("first claim: %+v err=%v", first, err)
	}
	if err := store.RenewAgentCommandLease(ctx, first.Command, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("current attempt renew: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE runtime_agent_commands SET lease_expires_at = ? WHERE idempotency_key = ?`,
		time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), command.IdempotencyKey); err != nil {
		t.Fatal(err)
	}
	second, err := store.ClaimAgentCommand(ctx, command)
	if err != nil || !second.Claimed || second.Command.AttemptToken == first.Command.AttemptToken {
		t.Fatalf("takeover: %+v err=%v", second, err)
	}
	if err := store.RenewAgentCommandLease(ctx, first.Command, time.Now().UTC().Add(time.Minute)); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("stale attempt renewed lease: %v", err)
	}
	first.Command.Status, first.Command.ResultState = "succeeded", contract.ReplicaReady
	if err := store.CompleteAgentCommand(ctx, first.Command); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("stale attempt completed command: %v", err)
	}
}

func seedElasticClaimBase(ctx context.Context, store *SQLiteStore, nodeID string) error {
	_, err := store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	if err != nil {
		return err
	}
	_, err = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err != nil {
		return err
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "seed",
	}); err != nil {
		return err
	}
	exp := time.Now().UTC().Add(time.Hour)
	return store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: nodeID, CapacityUnits: 4, Generation: 1, LeaseExpiry: exp,
	})
}

func mustSeedElasticClaimBase(t *testing.T, ctx context.Context, store *SQLiteStore, nodeID string) {
	t.Helper()
	if err := seedElasticClaimBase(ctx, store, nodeID); err != nil {
		t.Fatal(err)
	}
}

func mustClaimAssignment(t *testing.T, ctx context.Context, store *SQLiteStore, claim AssignmentClaim) {
	t.Helper()
	if err := store.ClaimAssignment(ctx, claim); err != nil {
		t.Fatalf("ClaimAssignment: %v", err)
	}
}

func mustRecordObservation(t *testing.T, ctx context.Context, store *SQLiteStore, obs ReplicaObservation) {
	t.Helper()
	if err := store.RecordObservation(ctx, obs); err != nil {
		t.Fatalf("RecordObservation: %v", err)
	}
}

func mustUpsertRuntimeNode(t *testing.T, ctx context.Context, store *SQLiteStore, node contract.RuntimeNode) {
	t.Helper()
	if err := store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatalf("UpsertRuntimeNode: %v", err)
	}
}

func mustElasticEnrolledReady(t *testing.T, ctx context.Context, store *SQLiteStore) {
	t.Helper()
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, ElasticEnrolled: true,
		BackgroundMode: contract.BackgroundModeNone,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCompareAndSetDesiredStrictGeneration(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/desire-strict.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 2, Reason: "bad",
	}); err != ErrDesiredCASConflict {
		t.Fatalf("want conflict on gen!=1 insert, got %v", err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, ServingDesireRow{
		DesiredReplicas: 2, Generation: 4, Reason: "skip",
	}); err != ErrDesiredCASConflict {
		t.Fatalf("want conflict on gen skip, got %v", err)
	}
}

func TestClaimAssignmentAndObservationFSM(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedElasticClaimBase(ctx, store, "n1"); err != nil {
		t.Fatal(err)
	}
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil)
	_ = store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	})

	valid := time.Now().UTC().Add(30 * time.Minute)
	claim := AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}
	if err := store.ClaimAssignment(ctx, claim); err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n2", CapacityUnits: 4, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour),
	})
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n2",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != ErrAssignmentCASConflict {
		t.Fatalf("want held assignment, got %v", err)
	}

	lease := valid
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting, AssignmentValidUntil: &lease,
	}); err != nil {
		t.Fatal(err)
	}
	epUntil := time.Now().UTC().Add(time.Hour)
	rev0, _ := store.GetRouteRevision(ctx)
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9901,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: &lease,
		EndpointValidUntil: &epUntil,
	}); err != nil {
		t.Fatal(err)
	}
	rev1, _ := store.GetRouteRevision(ctx)
	if rev1 <= rev0 {
		t.Fatalf("route revision should bump on ready: %d -> %d", rev0, rev1)
	}

	snap, ok, err := store.BuildSnapshotAfter(ctx, rev0)
	if err != nil || !ok {
		t.Fatalf("snapshot after: ok=%v err=%v", ok, err)
	}
	if len(snap.EndpointSets) != 1 || snap.EndpointSets[0].Endpoints[0].Address != "127.0.0.1:9901" {
		t.Fatalf("snap endpoints: %+v", snap.EndpointSets)
	}
}

func TestAtomicReadyRecoveryRevisionReplay(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/atomic-replay.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedElasticClaimBase(ctx, store, "n1"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{ProjectID: "demo", VersionID: "v1", Revision: 1, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := store.ClaimAssignment(ctx, AssignmentClaim{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, ReplicaObservation{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, State: contract.ReplicaStarting, AssignmentValidUntil: &valid}); err != nil {
		t.Fatal(err)
	}
	obs := ReplicaObservation{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, State: contract.ReplicaReady, ListenHost: "::1", ListenPort: 9901, EndpointState: contract.EndpointReady, AssignmentValidUntil: &valid, EndpointValidUntil: &valid}
	complete := func(key string, observation ReplicaObservation) {
		claim, err := store.ClaimAgentCommand(ctx, AgentCommand{IdempotencyKey: key, Action: contract.ActionStartReplica, NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "r1", Generation: 1})
		if err != nil {
			t.Fatal(err)
		}
		claim.Command.Status, claim.Command.ResultState = "succeeded", contract.ReplicaReady
		if err := store.RecordObservationAndCompleteAgentCommand(ctx, observation, claim.Command); err != nil {
			t.Fatal(err)
		}
	}
	complete("first", obs)
	rev1, _ := store.GetRouteRevision(ctx)
	complete("same", obs)
	rev2, _ := store.GetRouteRevision(ctx)
	if rev2 != rev1 {
		t.Fatalf("identical ready recovery bumped revision: %d -> %d", rev1, rev2)
	}
	changed := obs
	changed.ListenPort++
	complete("changed", changed)
	rev3, _ := store.GetRouteRevision(ctx)
	if rev3 != rev2+1 {
		t.Fatalf("changed endpoint did not bump revision: %d -> %d", rev2, rev3)
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil || len(snap.EndpointSets) != 1 || snap.EndpointSets[0].Endpoints[0].Address != "[::1]:9902" {
		t.Fatalf("IPv6 legacy snapshot: %+v err=%v", snap.EndpointSets, err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	view, ok, err := store.BuildQualificationViewAfter(ctx, -1)
	if err != nil || !ok || len(view.EndpointSets) != 1 || view.EndpointSets[0].Endpoints[0].Address != "[::1]:9902" {
		t.Fatalf("IPv6 qualification view: %+v ok=%v err=%v", view.EndpointSets, ok, err)
	}
}

func TestDeployReadyExcludedFromSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/deploy-ready.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil)
	_ = store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8801,
	})
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("deploy_ready must not be public ready: %+v", snap.EndpointSets)
	}
}

func TestExpiredLeaseFailClosed(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/lease.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	past := time.Now().UTC().Add(-time.Minute)
	if err := store.RenewRuntimeNodeLease(ctx, "n1", 1, past); err != ErrLeaseExpired {
		t.Fatalf("renew past expiry: %v", err)
	}
	if err := seedElasticClaimBase(ctx, store, "n1"); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(30 * time.Minute)
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		t.Fatal(err)
	}
	pastStr := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`UPDATE runtime_replicas SET valid_until = ? WHERE replica_id = ?`, pastStr, "r1"); err != nil {
		t.Fatal(err)
	}
	err = store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("want lease expired, got %v", err)
	}
}

func TestSetRouteBumpsRevisionInTransaction(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/route-tx.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	rev0, _ := store.GetRouteRevision(ctx)
	if err := store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8802,
	}); err != nil {
		t.Fatal(err)
	}
	rev1, _ := store.GetRouteRevision(ctx)
	if rev1 != rev0+1 {
		t.Fatalf("revision %d -> %d", rev0, rev1)
	}
}

func TestDesiredCASConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/cas.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "init",
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.CompareAndSetDesired(ctx, "demo", "v1", 1, ServingDesireRow{
				DesiredReplicas: 2, Generation: 2, Reason: "race",
			})
		}()
	}
	wg.Wait()
	close(errs)
	var conflicts int
	for err := range errs {
		if errors.Is(err, ErrDesiredCASConflict) {
			conflicts++
		} else if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("want exactly one CAS conflict, got %d", conflicts)
	}
}

func TestReconcileListQueries(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/reconcile.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedElasticClaimBase(ctx, store, "n1"); err != nil {
		t.Fatal(err)
	}
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil)
	_ = store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, ElasticEnrolled: true,
		BackgroundMode: contract.BackgroundModeNone,
	})
	valid := time.Now().UTC().Add(time.Hour)
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		t.Fatal(err)
	}
	vers, err := store.ListElasticEnrolledVersions(ctx)
	if err != nil || len(vers) != 1 {
		t.Fatalf("enrolled: %+v err=%v", vers, err)
	}
	reps, err := store.ListRuntimeReplicasForReconcile(ctx)
	if err != nil || len(reps) != 1 {
		t.Fatalf("reconcile reps: %+v err=%v", reps, err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	pastPtr := past
	_ = store.UpsertRuntimeReplica(ctx, contract.RuntimeReplica{
		ReplicaID: "r-old", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaPending, ValidUntil: &pastPtr,
	})
	expired, err := store.ListExpiredAssignments(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) < 1 {
		t.Fatalf("expected expired assignments, got %+v", expired)
	}
}

func TestUpsertRuntimeReplicaCompatibility(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/upsert.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	rep := contract.RuntimeReplica{
		ReplicaID: "legacy-upsert", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	}
	if err := store.UpsertRuntimeReplica(ctx, rep); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRuntimeReplica(ctx, "legacy-upsert")
	if err != nil || got == nil || got.State != contract.ReplicaStarting {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

func TestCompareAndSetElasticVersionStatus(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/version-status-cas.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 1, Generation: 1, Reason: "activator_ensure"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetElasticVersionStatus(ctx, "demo", "v1", StatusDeployReady, StatusReady, 1, 1); !errors.Is(err, ErrElasticVersionStatusCASConflict) {
		t.Fatalf("non-enrolled transition: %v", err)
	}
	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate prior deploy qualification; Activator may only re-warm a version
	// whose ready_at proves it previously completed the full qualification path.
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	rev0, _ := store.GetRouteRevision(ctx)
	if err := store.CompareAndSetElasticVersionStatus(ctx, "demo", "v1", StatusDeployReady, StatusReady, 1, 1); err != nil {
		t.Fatal(err)
	}
	rev1, _ := store.GetRouteRevision(ctx)
	if rev1 != rev0+1 {
		t.Fatalf("ready transition revision: %d -> %d", rev0, rev1)
	}
	if err := store.CompareAndSetElasticVersionStatus(ctx, "demo", "v1", StatusDeployReady, StatusReady, 1, 1); !errors.Is(err, ErrElasticVersionStatusCASConflict) {
		t.Fatalf("stale expected status: %v", err)
	}
	if err := store.SetProdVersionCAS(ctx, "demo", "", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, ServingDesireRow{DesiredReplicas: 0, Generation: 2, Reason: "idle"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetElasticVersionStatus(ctx, "demo", "v1", StatusReady, StatusDeployReady, 2, 0); !errors.Is(err, ErrElasticVersionStatusCASConflict) {
		t.Fatalf("prod scale-to-zero transition: %v", err)
	}
}
