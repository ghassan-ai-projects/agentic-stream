package transport

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Database struct {
	DB *storage.DB
}

func (d Database) Close() error {
	if d.DB == nil {
		return nil
	}
	return d.DB.Close() //nolint:wrapcheck // Raw storage close error; callers discard it on the session teardown path.
}

func OpenIsolatedDatabase(ctx context.Context, path string) (Database, error) {
	db, err := storage.OpenFresh(ctx, path)
	if err != nil {
		return Database{}, err //nolint:wrapcheck // App wraps with the session operation name.
	}
	return Database{DB: db}, nil
}

type SourceDatabase struct {
	DB *sql.DB
}

func (d SourceDatabase) Close() error {
	if d.DB == nil {
		return nil
	}
	if err := d.DB.Close(); err != nil {
		return fmt.Errorf("close source database: %w", err)
	}
	return nil
}

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
