package control_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

func TestReserveSettleAndKillSwitch(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	now := kernel.FormatTime(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	controller := control.CostLedger{}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return controltest.SetCostLimit(ctx, tx, "global", "", 10, false, now)
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
		return controltest.SetCostLimit(ctx, tx, "global", "", 10, true, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := controller.Reserve(ctx, tx, "episode-2", "tenant-1", 1, now); err == nil {
			return fmt.Errorf("expected kill switch rejection")
		} else if !errors.Is(err, control.ErrCostReservationRejected) {
			return fmt.Errorf("expected cost reservation rejection, got %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestZeroEstimateIsRejectedByTenantCeiling(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	now := kernel.FormatTime(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return controltest.SetCostLimit(ctx, tx, "tenant:tenant-1", "tenant-1", 10, false, now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := (control.CostLedger{}).Reserve(ctx, tx, "episode-1", "tenant-1", 0, now); err == nil {
			return fmt.Errorf("expected zero estimate rejection")
		} else if !errors.Is(err, control.ErrCostReservationRejected) {
			return fmt.Errorf("expected cost reservation rejection, got %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
