// Package app holds the notification use cases: appending an event (seal,
// deduplicate, allocate, insert), appending a lifecycle event, reading a page
// with resume, lag and poison handling, and pruning. It reaches storage only
// through the store layer and decisions only through domain.
package app
