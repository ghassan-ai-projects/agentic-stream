// Package store owns every notification SQL statement: notifications, cursors,
// event tombstones, poison attempts and audits. It selects and writes data;
// every decision lives in domain and app. Its unit of work is opaque and may
// be the caller's own transaction.
package store
