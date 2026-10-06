// Package store owns episode SQL and opaque transaction plumbing. Admission
// joins its caller-owned transaction; runner units of work own their claim and
// conclusion transactions. Ledger, cost and epoch ports receive
// the original transaction, with no domain decisions in the adapter.
package store
