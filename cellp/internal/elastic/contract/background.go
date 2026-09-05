package contract

// BackgroundCapability classifies whether a workload may scale to zero.
type BackgroundCapability string

const (
	CapabilityNone             BackgroundCapability = "none"
	CapabilityResidentRequired BackgroundCapability = "resident_required"
	CapabilityUnknown          BackgroundCapability = "unknown"
)

// WorkloadManifest is a versioned static manifest (WP-BG parses wrangler/bindings).
type WorkloadManifest struct {
	Revision     int            `json:"revision"`
	Capabilities map[string]BackgroundCapability `json:"capabilities"`
}
