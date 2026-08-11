// Package storage provides SQLite persistence for the runtime.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

// DB wraps a sql.DB with runtime-specific configuration.
type DB struct {
	*sql.DB
}

// Open opens or creates the SQLite database at path and runs pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	connStr := fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", path)
	sqlDB, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db := &DB{sqlDB}
	if err := db.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Migrate runs embedded migrations that have not yet been applied.
func (db *DB) Migrate(ctx context.Context) error {
	migrationList, err := migrations.All()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	var appliedVersions map[int]struct{}
	if hasMigrationsTable(ctx, db) {
		appliedVersions = make(map[int]struct{})
		rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
		if err != nil {
			return fmt.Errorf("list applied migrations: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err != nil {
				return fmt.Errorf("scan migration version: %w", err)
			}
			appliedVersions[v] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate migrations: %w", err)
		}
	}

	for _, m := range migrationList {
		if _, ok := appliedVersions[m.Version]; ok {
			continue
		}
		if err := db.runMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
	}

	return nil
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

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		m.Version, m.Name, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
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
