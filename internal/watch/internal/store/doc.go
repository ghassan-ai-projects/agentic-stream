// Package store owns the watch SQL (`watch_conditions`, `watch_fires`) and the
// owner and interlock plumbing on the original transaction. It selects and
// writes data; every decision lives in domain and app.
package store
