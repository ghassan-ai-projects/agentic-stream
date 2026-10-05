// Package transport is the gateway link to a device: a Unix-domain-socket
// connection carrying whole device records, with deadlines, cancellation and
// the "may have been sent" signal for partial writes. It does not interpret
// records.
package transport
