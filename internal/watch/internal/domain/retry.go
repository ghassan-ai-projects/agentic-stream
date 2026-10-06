package domain

import "time"

// Expiry is retried while the writer is contended: at most ExpireAttempts
// attempts, ExpireBackoff apart.
const (
	ExpireAttempts = 3
	ExpireBackoff  = 250 * time.Millisecond
)
