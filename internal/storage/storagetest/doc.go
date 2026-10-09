// Package storagetest is test support for the storage module: it opens a
// migrated runtime database in tests without replaying every migration. The
// migrated schema is built once per migration set and copied to each new
// path, so a test pays a file copy instead of a full migration run. Import it
// from tests only.
package storagetest
