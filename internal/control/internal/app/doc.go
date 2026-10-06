// Package app holds the control use cases: claiming, renewing and asserting the
// runtime owner lease, draining and killing epochs, reserving and settling
// episode cost, and the dispatch readiness gate. It reaches storage only
// through the store layer and decisions only through domain.
package app
