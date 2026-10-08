package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ConsumerName identifies the engine's inbox consumer.
const ConsumerName = "engine"

// OwnerCheck asserts runtime ownership of an epoch inside the transaction.
type OwnerCheck func(context.Context, *sql.Tx, string) error

// VersionProcessor reacts to a published Situation version inside the same
// transaction. It receives the original transaction so cognition commits or
// rolls back with the version.
type VersionProcessor interface {
	Process(context.Context, *sql.Tx, situations.Version) error
}

// Store keeps the database, the runtime ownership check and the engine's
// tenant and deployment scope private. It never exposes a raw transaction.
type Store struct {
	db           *storage.DB
	owner        OwnerCheck
	epoch        string
	tenantID     string
	deploymentID string
}

// Tx is an opaque unit of work scoped to one tenant and deployment.
type Tx struct {
	tx           *sql.Tx
	owner        OwnerCheck
	epoch        string
	tenantID     string
	deploymentID string
}

// New binds the persistence ports and scope without opening a transaction.
func New(db *storage.DB, owner OwnerCheck, epoch, tenantID, deploymentID string) Store {
	return Store{db: db, owner: owner, epoch: epoch, tenantID: tenantID, deploymentID: deploymentID}
}

// Configured reports whether every persistence safety port was supplied.
func (s Store) Configured() bool { return s.db != nil && s.owner != nil }

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		return use(&Tx{tx: tx, owner: s.owner, epoch: s.epoch, tenantID: s.tenantID, deploymentID: s.deploymentID})
	})
}

// RetryBusy retries fn while SQLite reports writer contention.
func (s Store) RetryBusy(ctx context.Context, fn func() error) error {
	return storage.RetrySQLiteBusy(ctx, fn) //nolint:wrapcheck // The retry helper owns the contention error text.
}

// CheckpointWAL folds the write-ahead log into the database, tolerating
// contention.
func (s Store) CheckpointWAL(ctx context.Context) error {
	if err := s.db.Checkpoint(ctx); err != nil && !storage.IsSQLiteBusy(err) {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	return nil
}

// SaveDeployment records the compiled spec as this tenant's deployment.
func (s Store) SaveDeployment(ctx context.Context, compiled *spec.CompiledSpec) error {
	if err := spec.SaveDeployment(ctx, s.db, s.tenantID, compiled); err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	return nil
}

// LoadCheckpoint reads a partition's progress; a partition never seen has the
// zero checkpoint.
func (s Store) LoadCheckpoint(ctx context.Context, partitionID int) (domain.Checkpoint, error) {
	var result domain.Checkpoint
	var watermark sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT last_position, watermark FROM partition_checkpoints WHERE consumer_name = ? AND tenant_id = ? AND partition_id = ?",
		ConsumerName, s.tenantID, partitionID,
	).Scan(&result.LastPosition, &watermark)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, fmt.Errorf("query checkpoint: %w", err)
	}
	if watermark.Valid {
		result.Watermark = watermark.String
	}
	return result, nil
}

// AppliedThrough is the log position up to which the global run has applied
// every record: records are applied in position order and positions commit in
// order (SQLite serializes writers), so the highest partition checkpoint is a
// position below which nothing is left to apply. Zero before the first record.
func (s Store) AppliedThrough(ctx context.Context) (int64, error) {
	var position int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(last_position), 0) FROM partition_checkpoints WHERE consumer_name = ? AND tenant_id = ?",
		ConsumerName, s.tenantID,
	).Scan(&position); err != nil {
		return 0, fmt.Errorf("query applied position: %w", err)
	}
	return position, nil
}

// AssertOwner requires the configured runtime owner and epoch in this transaction.
func (tx *Tx) AssertOwner(ctx context.Context) error {
	if err := tx.owner(ctx, tx.tx, tx.epoch); err != nil {
		return fmt.Errorf("stream runtime ownership lost: %w", err)
	}
	return nil
}

// ProcessVersion lets cognition react to a version in this transaction.
func (tx *Tx) ProcessVersion(ctx context.Context, processor VersionProcessor, version situations.Version) error {
	return processor.Process(ctx, tx.tx, version) //nolint:wrapcheck // The caller names the failed step.
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
