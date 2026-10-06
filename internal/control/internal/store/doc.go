// Package store owns every control SQL statement: the runtime owner lease, the
// epoch control record, cost limits and cost reservations. It selects and
// writes data; every decision lives in domain and app. Its unit of work is
// opaque and may be the caller's own transaction.
package store
