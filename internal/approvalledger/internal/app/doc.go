// Package app holds the approval lifecycle use cases: request, expire, resolve,
// bind an assertion, withdraw, and withdraw every approval a newer Situation
// version superseded, publishing each withdrawal through the caller's
// publisher in the same transaction. It reaches storage only through the store
// layer.
package app
