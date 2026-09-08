package registry

// IsSQLiteBusy reports SQLite BUSY/LOCKED errors (including extended result codes).
// Unstable: for in-repo cross-package test observation retries only; not a supported public API contract.
func IsSQLiteBusy(err error) bool {
	return isBusy(err)
}
