// Package app orders the run artifact use cases: export one consistent
// snapshot of a run as an immutable directory, and verify such a directory. It
// decides through domain, reads through store and writes or reads files through
// transport; it holds no SQL and no file-system calls.
package app
