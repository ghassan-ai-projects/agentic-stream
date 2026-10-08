package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/domain"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

// DB wraps a sql.DB with runtime-specific configuration.
type DB struct {
	*sql.DB
	reservationPath string
}

// Open opens or creates the SQLite database at path and runs pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if _, err := os.Lstat(domain.ReservationPath(path)); err == nil {
		return nil, fmt.Errorf("database path is reserved by an active replay: %s", path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect replay reservation: %w", err)
	}
	return open(ctx, path)
}

// OpenFresh atomically reserves a new database path before opening SQLite.
// It is used by isolated replay so an existing database, symlink, or
// concurrent creator cannot be mistaken for a disposable run database.
func OpenFresh(ctx context.Context, path string) (*DB, error) {
	reservationPath, err := reserveFreshDatabase(path)
	if err != nil {
		return nil, err
	}
	db, err := open(ctx, path)
	if err != nil {
		_ = os.Remove(reservationPath)
		return nil, err
	}
	db.reservationPath = reservationPath
	return db, nil
}

// Close closes the database and releases its private replay reservation.
func (db *DB) Close() error {
	err := db.DB.Close()
	if db.reservationPath != "" {
		if cleanupErr := os.Remove(db.reservationPath); err == nil {
			err = cleanupErr
		}
	}
	return err
}

func open(ctx context.Context, path string) (*DB, error) {
	if err := requireStoredTimeFunction(); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", domain.ConnectionString(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db := &DB{DB: sqlDB}
	if err := db.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Checkpoint performs a non-blocking WAL checkpoint. SQLite may leave frames
// for a later checkpoint when readers or another writer are active; callers
// should treat that as normal maintenance behavior.
func (db *DB) Checkpoint(ctx context.Context) error {
	var busy, logFrames, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	return nil
}

// Migrate runs embedded migrations that have not yet been applied.
func (db *DB) Migrate(ctx context.Context) error {
	migrationList, err := migrations.All()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	applied, err := db.appliedMigrationVersions(ctx)
	if err != nil {
		return err
	}
	for _, m := range domain.PendingMigrations(migrationList, applied) {
		if err := db.runMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// appliedMigrationVersions returns the recorded migration versions; a new
// database has none.
func (db *DB) appliedMigrationVersions(ctx context.Context) (map[int]struct{}, error) {
	if !hasMigrationsTable(ctx, db) {
		return make(map[int]struct{}), nil
	}
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanMigrationVersions(rows)
}

func scanMigrationVersions(rows *sql.Rows) (map[int]struct{}, error) {
	applied := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return applied, nil
}

func hasMigrationsTable(ctx context.Context, db *DB) bool {
	var name string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'",
	).Scan(&name); err != nil {
		return false
	}
	return name == "schema_migrations"
}

func (db *DB) runMigration(ctx context.Context, m migrations.Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := applyMigration(ctx, tx, m); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}

// applyMigration executes the migration script and records its version.
func applyMigration(ctx context.Context, tx *sql.Tx, m migrations.Migration) error {
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		m.Version, m.Name, kernel.FormatTime(time.Now()),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return nil
}

// WithTx runs fn inside a transaction that commits if fn returns nil.
func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
