// Package store owns every SQL statement on the approvals table. It also reads
// the intents and decisions that bind an approval to a Situation version, which
// belong to policy. It selects and writes data; the lifecycle order lives in
// app and the vocabulary in domain. Its unit of work is always the caller's
// transaction.
package store
