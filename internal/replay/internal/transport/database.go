package transport

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // read-only source database driver

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Database is the isolated replay database resource. The application layer
// holds and closes it without importing the storage package.
type Database struct {
	DB *storage.DB
}

// Close releases the isolated database.
func (d Database) Close() error {
	if d.DB == nil {
		return nil
	}
	return d.DB.Close() //nolint:wrapcheck // Raw storage close error; callers discard it on the session teardown path.
}

// OpenIsolatedDatabase creates the fresh database one replay session owns.
func OpenIsolatedDatabase(ctx context.Context, path string) (Database, error) {
	db, err := storage.OpenFresh(ctx, path)
	if err != nil {
		return Database{}, err //nolint:wrapcheck // App wraps with the session operation name.
	}
	return Database{DB: db}, nil
}

// SourceDatabase is a live runtime database opened read-only, the recorded
// ledger of a recorded replay. Replay never writes to it.
type SourceDatabase struct {
	DB *sql.DB
}

// Close releases the source database.
func (d SourceDatabase) Close() error {
	if d.DB == nil {
		return nil
	}
	return d.DB.Close() //nolint:wrapcheck // Raw close error; callers discard it on teardown.
}

// OpenSourceDatabase opens a runtime database read-only: no migration, no
// write, so recording a replay cannot change the evidence it reads.
func OpenSourceDatabase(ctx context.Context, path string) (SourceDatabase, error) {
	if _, err := os.Stat(path); err != nil {
		return SourceDatabase{}, fmt.Errorf("source database: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return SourceDatabase{}, fmt.Errorf("open source database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return SourceDatabase{}, fmt.Errorf("open source database: %w", err)
	}
	return SourceDatabase{DB: db}, nil
}
