package costcontrol_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestReserveSettleAndKillSwitch(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	controller := costcontrol.Controller{}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return costcontrol.SetLimit(ctx, tx, "global", "", 10, false, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return controller.Reserve(ctx, tx, "episode-1", "tenant-1", 7, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := controller.Settle(ctx, tx, "episode-1", 6, now); err != nil {
			return fmt.Errorf("settle episode: %w", err)
		}
		return costcontrol.SetLimit(ctx, tx, "global", "", 10, true, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := controller.Reserve(ctx, tx, "episode-2", "tenant-1", 1, now); err == nil {
			return fmt.Errorf("expected kill switch rejection")
		} else if !errors.Is(err, costcontrol.ErrReservationRejected) {
			return fmt.Errorf("expected cost reservation rejection, got %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestZeroEstimateIsRejectedByTenantCeiling(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return costcontrol.SetLimit(ctx, tx, "tenant:tenant-1", "tenant-1", 10, false, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := (costcontrol.Controller{}).Reserve(ctx, tx, "episode-1", "tenant-1", 0, now); err == nil {
			return fmt.Errorf("expected zero estimate rejection")
		} else if !errors.Is(err, costcontrol.ErrReservationRejected) {
			return fmt.Errorf("expected cost reservation rejection, got %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
