package orch

import "context"

// IngressListenerReconciler opens/closes dedicated ingress TCP listeners (P5c).
type IngressListenerReconciler interface {
	ReconcileIngressListeners(ctx context.Context) error
}
