package contract

import "time"

const (
	// MaxControlPlaneScopeTTL bounds IssuedAt→ExpiresAt for node registration and registry relay scopes.
	MaxControlPlaneScopeTTL = 2 * time.Minute
	// MaxControlPlaneClockSkew tolerates minor clock skew when validating IssuedAt.
	MaxControlPlaneClockSkew = 30 * time.Second
)

// validateControlPlaneScopeWindow enforces TTL and clock skew for replay-bounded scopes.
func validateControlPlaneScopeWindow(issued, expires, now time.Time, expiredMsg string) error {
	now = now.UTC()
	issued = issued.UTC()
	expires = expires.UTC()
	if !expires.After(issued) {
		return errScopeWindow("expires_at must be after issued_at")
	}
	if expires.Sub(issued) > MaxControlPlaneScopeTTL {
		return errScopeWindow("scope ttl exceeds maximum")
	}
	if issued.After(now.Add(MaxControlPlaneClockSkew)) {
		return errScopeWindow("issued_at in future")
	}
	if !expires.After(now) {
		return errScopeWindow(expiredMsg)
	}
	return nil
}

type errScopeWindow string

func (e errScopeWindow) Error() string { return string(e) }
