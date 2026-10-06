// Package store owns every run artifact SQL statement. All reads happen in one
// read-only snapshot transaction. It selects data; every decision lives in
// domain and app. The module owns no tables.
package store
