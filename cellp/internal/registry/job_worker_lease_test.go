package registry

import (
	"database/sql"
	"testing"
	"time"
)

func nullLease(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func TestJobWorkerLeaseTruthTable(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute).Format(time.RFC3339Nano)
	past := now.Add(-time.Minute).Format(time.RFC3339Nano)

	cases := []struct {
		name      string
		lease     sql.NullString
		isLive    bool
		renewable bool
		needs     bool
		blocked   bool
	}{
		{"future-valid", nullLease(future), true, true, false, false},
		{"expired-valid", nullLease(past), false, false, true, false},
		{"null", sql.NullString{}, false, false, true, true},
		{"empty", nullLease(""), false, false, true, true},
		{"invalid", nullLease("not-rfc3339"), true, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jobWorkerLeaseIsLive(tc.lease, now); got != tc.isLive {
				t.Fatalf("IsLive: got %v want %v", got, tc.isLive)
			}
			if got := jobWorkerLeaseRenewable(tc.lease, now); got != tc.renewable {
				t.Fatalf("Renewable: got %v want %v", got, tc.renewable)
			}
			if got := jobWorkerLeaseNeedsCompensationRepair(tc.lease, now); got != tc.needs {
				t.Fatalf("NeedsRepair: got %v want %v", got, tc.needs)
			}
			if got := jobWorkerLeaseClaimTakeoverBlocked(tc.lease); got != tc.blocked {
				t.Fatalf("TakeoverBlocked: got %v want %v", got, tc.blocked)
			}
		})
	}
}
