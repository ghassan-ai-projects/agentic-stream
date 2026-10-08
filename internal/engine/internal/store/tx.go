package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const ConsumerName = "engine"

type OwnerCheck func(context.Context, *sql.Tx, string) error

type VersionProcessor interface {
	Process(context.Context, *sql.Tx, situations.Version) error
}

type Store struct {
	db           *storage.DB
	owner        OwnerCheck
	epoch        string
	tenantID     string
	deploymentID string
}

type Tx struct {
	tx           *sql.Tx
	owner        OwnerCheck
	epoch        string
	tenantID     string
	deploymentID string
}

func New(db *storage.DB, owner OwnerCheck, epoch, tenantID, deploymentID string) Store {
	return Store{db: db, owner: owner, epoch: epoch, tenantID: tenantID, deploymentID: deploymentID}
}

func (s Store) Configured() bool { return s.db != nil && s.owner != nil }

func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		return use(&Tx{tx: tx, owner: s.owner, epoch: s.epoch, tenantID: s.tenantID, deploymentID: s.deploymentID})
	})
}

func (s Store) RetryBusy(ctx context.Context, fn func() error) error {
	return storage.RetrySQLiteBusy(ctx, fn) //nolint:wrapcheck // The retry helper owns the contention error text.
}

func (s Store) CheckpointWAL(ctx context.Context) error {
	if err := s.db.Checkpoint(ctx); err != nil && !storage.IsSQLiteBusy(err) {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	return nil
}

func (s Store) SaveDeployment(ctx context.Context, compiled *spec.CompiledSpec) error {
	if err := spec.SaveDeployment(ctx, s.db, s.tenantID, compiled); err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	return nil
}

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

func (tx *Tx) AssertOwner(ctx context.Context) error {
	if err := tx.owner(ctx, tx.tx, tx.epoch); err != nil {
		return fmt.Errorf("stream runtime ownership lost: %w", err)
	}
	return nil
}

func (tx *Tx) ProcessVersion(ctx context.Context, processor VersionProcessor, version situations.Version) error {
	return processor.Process(ctx, tx.tx, version) //nolint:wrapcheck // The caller names the failed step.
}
