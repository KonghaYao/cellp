package contract

import "fmt"

// BackgroundGuardOptions tunes fail-closed replica bounds (WP-SCALE / WP-BG).
type BackgroundGuardOptions struct {
	// MultiReplicaBackgroundProven is true only after SP-E3 evidence (default false).
	MultiReplicaBackgroundProven bool
}

// ValidateServingPolicyBackground enforces resident_required and multi-replica caps.
func ValidateServingPolicyBackground(p ServingPolicy, opts BackgroundGuardOptions) error {
	if err := ValidateServingPolicy(p); err != nil {
		return err
	}
	if p.BackgroundMode == BackgroundModeResidentRequired {
		if p.MinReplicas < 1 {
			return fmt.Errorf("resident_required requires min_replicas >= 1")
		}
	}
	if !opts.MultiReplicaBackgroundProven {
		if p.BackgroundMode == BackgroundModeResidentRequired && p.MaxReplicas > 1 {
			return fmt.Errorf("resident_required max_replicas must be 1 until SP-E3 multi-replica proof")
		}
	}
	return nil
}
