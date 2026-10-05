package transport

import (
	"context"

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
