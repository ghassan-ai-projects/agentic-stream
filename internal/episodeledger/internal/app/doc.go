// Package app holds the episode pipeline use cases: scheduler queue operations,
// episode admission, attempt start and transition under fencing, worker
// identity validation, rejection audit, supersession and recovery. Every
// operation runs on the caller's transaction. It reaches storage only through
// the store layer and decisions only through domain.
package app
