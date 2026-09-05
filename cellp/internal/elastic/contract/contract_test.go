package contract

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestParentBranchEligible(t *testing.T) {
	if !ParentBranchEligible(StatusReady) || !ParentBranchEligible(StatusArchived) || !ParentBranchEligible(StatusDeployReady) {
		t.Fatal("expected branchable")
	}
	if ParentBranchEligible(StatusPending) {
		t.Fatal("pending must not branch")
	}
}

func TestValidVersionStatus_includesDeployReady(t *testing.T) {
	if !ValidVersionStatus(StatusDeployReady) {
		t.Fatal("deploy_ready must be valid")
	}
}

func TestValidateServingPolicy(t *testing.T) {
	ok := ServingPolicy{MinReplicas: 0, MaxReplicas: 2, BackgroundMode: BackgroundModeNone}
	if err := ValidateServingPolicy(ok); err != nil {
		t.Fatal(err)
	}
	bad := ServingPolicy{MinReplicas: 2, MaxReplicas: 1, BackgroundMode: BackgroundModeNone}
	if err := ValidateServingPolicy(bad); err == nil {
		t.Fatal("expected max < min error")
	}
	if err := ValidateServingPolicy(ServingPolicy{BackgroundMode: BackgroundModeUnknown}); err == nil {
		t.Fatal("unknown background fail-closed")
	}
}

func TestValidateRouteSnapshot_revision(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	snap := RouteSnapshot{
		Revision: 2,
		EndpointSets: []EndpointSet{{
			ProjectID: "p",
			VersionID: "v",
			Endpoints: []Endpoint{{ReplicaID: "r1", Address: "127.0.0.1:1", State: EndpointReady, ValidUntil: &future}},
		}},
	}
	if err := ValidateRouteSnapshot(1, snap, now); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRouteSnapshot(2, snap, now); err == nil {
		t.Fatal("expected regression error")
	}
}

func TestElasticRuntimeEnabled_defaultOff(t *testing.T) {
	t.Setenv(EnvElasticRuntime, "")
	if ElasticRuntimeEnabled() {
		t.Fatal("default off")
	}
	t.Setenv(EnvElasticRuntime, "1")
	if !ElasticRuntimeEnabled() {
		t.Fatal("1 should enable")
	}
}

func TestRouteSnapshotJSONGolden(t *testing.T) {
	snap := RouteSnapshot{
		Revision:       1,
		PolicyRevision: 1,
		Bindings: []IngressBinding{{
			Role: "preview", Host: "preview.example", ListenPort: 19080,
			ProjectID: "demo", VersionID: "v1",
		}},
		EndpointSets: []EndpointSet{{
			ProjectID: "demo", VersionID: "v1",
			Endpoints: []Endpoint{{ReplicaID: "r0", Address: "127.0.0.1:8792", State: EndpointReady}},
		}},
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const golden = "testdata/route_snapshot_v1.golden.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 go test): %v", err)
	}
	if string(b) != string(want) {
		t.Fatalf("golden mismatch\n--- got\n%s", string(b))
	}
}
