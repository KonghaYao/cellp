package autoscaler

import "github.com/cellp/cellp/internal/elastic/contract"

// CountReadyReplicas counts replicas in ready state.
func CountReadyReplicas(replicas []contract.RuntimeReplica) int {
	n := 0
	for _, r := range replicas {
		if r.State == contract.ReplicaReady {
			n++
		}
	}
	return n
}
