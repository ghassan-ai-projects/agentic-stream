// Package domain holds the persistence rules as pure code: the SQLite connection
// string, the busy-retry backoff schedule, which migrations are still pending and
// the names a fresh replay database reserves. It opens no file and reads no clock.
package domain
