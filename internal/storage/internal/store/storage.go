package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/domain"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

type DB struct {
	*sql.DB
	reservationPath string
}

func Open(ctx context.Context, path string) (*DB, error) {
	if _, err := os.Lstat(domain.ReservationPath(path)); err == nil {
		return nil, fmt.Errorf("database path is reserved by an active replay: %s", path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect replay reservation: %w", err)
	}
	return open(ctx, path)
}

func OpenExisting(ctx context.Context, path string) (*DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("runtime database %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("runtime database %s is not a regular file", path)
	}
	return Open(ctx, path)
}

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
	sqlDB, err := openPool(path)
	if err != nil {
		return nil, err
	}
	db := &DB{DB: sqlDB}
	if err := db.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func openPool(path string) (*sql.DB, error) {
	sqlDB, err := sql.Open("sqlite", domain.ConnectionString(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(domain.MaxOpenConnections)
	sqlDB.SetMaxIdleConns(domain.MaxOpenConnections)
	return sqlDB, nil
}

func (db *DB) Migrate(ctx context.Context) error {
	migrationList, err := migrations.All()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	applied, err := db.appliedMigrationVersions(ctx)
	if err != nil {
		return err
	}
	return db.applyPending(ctx, domain.PendingMigrations(migrationList, applied), len(applied))
}

func (db *DB) applyPending(ctx context.Context, pending []migrations.Migration, alreadyApplied int) error {
	for _, m := range pending {
		if err := db.runMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
	}
	if len(pending) > 0 {
		slog.Info("database migrated", "applied", len(pending), "from_version", alreadyApplied, "to_version", pending[len(pending)-1].Version)
	}
	return nil
}

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

func (db *DB) BackupInto(ctx context.Context, path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("backup target %s already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect backup target %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("back up database into %s: %w", path, err)
	}
	return nil
}

func (db *DB) Vacuum(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}
