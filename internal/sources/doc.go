// Package sources holds the runtime's injected sources of non-determinism: time
// (a physical clock and a virtual one for replay) and identifiers (a random
// generator and a deterministic one for replay and tests), plus the identity
// prefixes. Rules receive them as parameters and never read the wall clock or a
// random source themselves. The facade exposes what other packages use; the
// vocabulary, the virtual clock and the deterministic generator live in
// internal/domain, and the operating-system clock and random generator in
// internal/transport.
package sources
