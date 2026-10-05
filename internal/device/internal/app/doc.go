// Package app holds the device-boundary use cases: the device session
// (handshake, state query, exchange, safe-stop lane, reconciliation, output
// verification) and the three effectors. It applies the domain rules, talks to
// the gateway only through the Transport port and the wire codec, and records
// durable facts through the device authority.
package app
