// Package store persists the device-authority tables: target claims, command
// bindings, reconciliation, authority events and safety events. It holds every
// SQL statement for those tables and maps rows to and from domain values. It
// makes no decisions: mutations take the caller's transaction, and reads take
// a Reader so they can run inside that transaction or on their own.
package store
