package contract

import (
	"fmt"
	"strings"
)

// ValidateNodeIdentityAllowlist ensures node_id↔SPIFFE URI is a bijection with valid URIs.
func ValidateNodeIdentityAllowlist(allowlist map[string]string) error {
	if len(allowlist) == 0 {
		return fmt.Errorf("node identity allowlist required")
	}
	seenURI := make(map[string]string, len(allowlist))
	for nodeID, uri := range allowlist {
		nodeID = strings.TrimSpace(nodeID)
		uri = strings.TrimSpace(uri)
		if nodeID == "" {
			return fmt.Errorf("node_id required in allowlist")
		}
		if len(nodeID) > MaxCommandFieldLen {
			return fmt.Errorf("node_id too long in allowlist")
		}
		if err := validateSPIFFEIdentityURI(uri); err != nil {
			return fmt.Errorf("node %q: %w", nodeID, err)
		}
		if other, ok := seenURI[uri]; ok && other != nodeID {
			return fmt.Errorf("duplicate spiffe uri for nodes %q and %q", other, nodeID)
		}
		seenURI[uri] = nodeID
	}
	return nil
}
