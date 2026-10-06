// Package store holds every event-log SQL statement and transaction. It
// exposes a Unit for caller-owned transactions so use cases can span
// admission, insert and lifecycle steps atomically; methods are named after
// domain actions and decide nothing.
package store
