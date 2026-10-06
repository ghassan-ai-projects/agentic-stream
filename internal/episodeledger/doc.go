// Package episodeledger owns the durable episode pipeline lifecycle: scheduler
// queue items (identity, admission, coalescing, skipped opportunities) and the
// episodes they admit (admission identity, attempts, fencing, transitions,
// rejection audit, supersession and recovery mutations), all scoped to the
// caller's transaction.
package episodeledger
