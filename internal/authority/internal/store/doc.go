// Package store persists the device-authority tables: target claims, command
// bindings, reconciliation, authority events and safety events. It owns the
// module's transactions: a Tx is one unit of work, opened either behind the
// ordinary-admission fence or on the priority path. Every SQL statement for
// those tables lives here, mapping rows to and from domain values; the store
// makes no decisions.
package store
