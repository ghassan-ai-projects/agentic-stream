package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type interlockOperation func(context.Context, *storage.DB, string) (interlock.State, error)

func newInterlockCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "interlock", Short: "Show, trip or clear the global action-plane interlock."}
	cmd.AddCommand(
		newInterlockOperationCommand("status", "Show the interlock.", false, readInterlock),
		newInterlockOperationCommand("trip", "Block every effect until the interlock is cleared (no runtime lease needed).", true, tripInterlock),
		newInterlockOperationCommand("clear", "Reopen the action plane (the runtime must be stopped).", true, clearInterlock),
	)
	return cmd
}

func newInterlockOperationCommand(name, short string, needsReason bool, operation interlockOperation) *cobra.Command {
	var reason string
	return newListCommand(name, short,
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
			return runInterlock(cmd, flags, db, operation, reason)
		},
		func(cmd *cobra.Command) {
			if needsReason {
				cmd.Flags().StringVar(&reason, "reason", "", "Why the interlock changes (required)")
			}
		})
}

func readInterlock(ctx context.Context, db *storage.DB, _ string) (interlock.State, error) {
	state, err := interlock.Status(ctx, db)
	if err != nil {
		return interlock.State{}, fmt.Errorf("interlock status: %w", err)
	}
	return state, nil
}

func tripInterlock(ctx context.Context, db *storage.DB, reason string) (interlock.State, error) {
	state, err := interlock.Trip(ctx, db, reason, time.Now())
	if err != nil {
		return interlock.State{}, fmt.Errorf("interlock trip: %w", err)
	}
	return state, nil
}

func clearInterlock(ctx context.Context, db *storage.DB, reason string) (interlock.State, error) {
	var state interlock.State
	err := withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) (err error) {
		fence := func(ctx context.Context, tx *sql.Tx) error { return ownership.owner.Assert(ctx, tx, ownership.epoch) }
		state, err = interlock.Clear(ctx, db, fence, reason, time.Now())
		if err != nil {
			return fmt.Errorf("interlock clear: %w", err)
		}
		return nil
	})
	return state, err
}

func runInterlock(cmd *cobra.Command, flags operatorFlags, db *storage.DB, operation interlockOperation, reason string) error {
	state, err := operation(cmd.Context(), db, reason)
	if err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, state, func() string {
		return fmt.Sprintf("interlock: %s (version %d, %s) %s", state.Status, state.Version, state.UpdatedAt, state.Reason)
	})
}
