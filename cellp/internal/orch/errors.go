package orch

import "errors"

// ErrOffshootPromote is returned when offshoot promote fails during the promote saga (AD-5).
var ErrOffshootPromote = errors.New("offshoot_promote_failed")
