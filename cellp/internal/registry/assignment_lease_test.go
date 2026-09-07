package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestClaimAssignmentActiveLimitIsAtomic(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/assignment-limit.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: expiry,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/n1", Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	claims := []AssignmentClaim{
		{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 1, ValidUntil: expiry},
		{ReplicaID: "r2", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 1, ValidUntil: expiry},
	}
	var wg sync.WaitGroup
	results := make(chan error, len(claims))
	for _, claim := range claims {
		claim := claim
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.ClaimAssignment(ctx, claim)
		}()
	}
	wg.Wait()
	close(results)
	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAssignmentCASConflict):
			conflicted++
		default:
			t.Fatalf("claim: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestRenewAssignmentLeaseFencedAndAtomic(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/renew-assignment.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	nodeExpiry := now.Add(time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 1, LeaseExpiry: nodeExpiry,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/n1", Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	oldExpiry := now.Add(10 * time.Minute)
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: oldExpiry,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1,
		State: contract.ReplicaStarting, AssignmentValidUntil: &oldExpiry,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1,
		State: contract.ReplicaReady, ListenHost: "127.0.0.1", ListenPort: 9901,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: &oldExpiry, EndpointValidUntil: &oldExpiry,
	}); err != nil {
		t.Fatal(err)
	}
	newExpiry := now.Add(30 * time.Minute)
	if err := store.RenewAssignmentLease(ctx, "r1", "n1", 1, 2, oldExpiry, newExpiry); !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("stale node generation renewal: %v", err)
	}
	rep, _ := store.GetRuntimeReplica(ctx, "r1")
	if rep.ValidUntil == nil || !rep.ValidUntil.Equal(oldExpiry) {
		t.Fatalf("failed renewal mutated assignment: %+v", rep)
	}
	if err := store.RenewAssignmentLease(ctx, "r1", "n1", 1, 1, oldExpiry, newExpiry); err != nil {
		t.Fatal(err)
	}
	rep, _ = store.GetRuntimeReplica(ctx, "r1")
	if rep.ValidUntil == nil || !rep.ValidUntil.Equal(newExpiry) {
		t.Fatalf("assignment expiry: %+v", rep)
	}
	var endpointExpiry string
	if err := store.db.QueryRowContext(ctx, `SELECT valid_until FROM runtime_replica_endpoints WHERE replica_id = ?`, "r1").Scan(&endpointExpiry); err != nil {
		t.Fatal(err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, endpointExpiry)
	if err != nil || !parsed.Equal(newExpiry) {
		t.Fatalf("endpoint expiry=%s err=%v", endpointExpiry, err)
	}
	if err := store.RenewAssignmentLease(ctx, "r1", "n1", 1, 1, oldExpiry, now.Add(40*time.Minute)); !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("stale expiry renewal: %v", err)
	}
}

func TestClaimAssignmentExpiredReplicasDoNotCountTowardActiveLimit(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/expired-active.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	past := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: future,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/n1", Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 1, ValidUntil: future,
	}); err != nil {
		t.Fatal(err)
	}
	pastStr := past.Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`UPDATE runtime_replicas SET valid_until = ? WHERE replica_id = ?`, pastStr, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r2", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 1, ValidUntil: future,
	}); err != nil {
		t.Fatalf("expired row should not block active limit: %v", err)
	}
}

func TestClaimAssignmentNodeCapacityIsAtomic(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/node-cap.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{DesiredReplicas: 2, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1, LeaseExpiry: expiry,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/n1", Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	claims := []AssignmentClaim{
		{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 2, ValidUntil: expiry},
		{ReplicaID: "r2", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ActiveLimit: 2, ValidUntil: expiry},
	}
	var wg sync.WaitGroup
	results := make(chan error, len(claims))
	for _, claim := range claims {
		claim := claim
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.ClaimAssignment(ctx, claim)
		}()
	}
	wg.Wait()
	close(results)
	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAssignmentCASConflict):
			conflicted++
		default:
			t.Fatalf("claim: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("node capacity: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}
