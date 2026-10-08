// Package kernel holds the project's shared pure vocabulary: representations
// that every module must spell the same way and that need no state, no clock
// read and no I/O. Any production package may import it without declaring the
// edge; in return it imports only the standard library, never touches a
// database, file, network or random source, and never reads the wall clock.
// A symbol belongs here only when two or more modules need it and it is a
// stable representation rule, not business behavior.
package kernel
