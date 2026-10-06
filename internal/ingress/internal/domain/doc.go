// Package domain holds ingress rules as pure functions over values: envelope
// admission, quarantine and connector identities, the checkpoint codec, the
// streams-simulator trace grammar and event conversion (with the embedded
// channel-to-field data), and the live socket path rule. It performs no I/O and
// reads no clock.
package domain
