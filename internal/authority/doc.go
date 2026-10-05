// Package authority owns device authority: whether this runtime may send an
// ordinary command to a physical target right now, and the durable evidence of
// every answer. It covers target claims and fences, command bindings, device
// reconciliation, safe-stop latching and safety evidence.
//
// Service is the only entry point. Each operation validates its input, runs in
// one transaction that starts with ordinary admission (or deliberately skips
// it on the priority path), loads state through the store layer, decides with
// the pure domain layer, persists, and audits. The domain and store layers
// live under internal/ and cannot be imported from outside this package.
package authority
