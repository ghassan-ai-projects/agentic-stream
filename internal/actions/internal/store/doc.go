// Package store owns the action plane's SQL: the command, outbox, outcome and
// verification ledgers, read-only projections of the rows a command's authority
// rests on, and the same-transaction owner, interlock, authority and
// notification plumbing. It selects and writes data; every decision lives in
// domain and app.
package store
