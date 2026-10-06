// Package store owns the interlock SQL (`runtime_interlock`, one row). It reads
// and writes the row on the caller's transaction; the decisions live in domain.
package store
