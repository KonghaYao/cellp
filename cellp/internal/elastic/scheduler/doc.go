// Package scheduler is the AD-15 assignment writer and controller-side lifecycle reconciler.
// It reads serving desires, places generation-fenced assignments, and drives Node Agent
// Start/Probe/Drain/Stop over HTTPS+mTLS. It must not call runtime.Manager directly.
package scheduler
