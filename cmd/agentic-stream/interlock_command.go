package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// interlockOperation reads or changes the interlock on an open database.
type interlockOperation func(context.Context, *storage.DB, string) (runtimecontrol.InterlockState, error)

func newInterlockCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "interlock", Short: "Show, trip or clear the global action-plane interlock."}
	cmd.AddCommand(
		newInterlockOperationCommand("status", "Show the interlock.", false, readInterlock),
		newInterlockOperationCommand("trip", "Block every effect until the interlock is cleared (no runtime lease needed).", true, runtimecontrol.TripInterlock),
		newInterlockOperationCommand("clear", "Reopen the action plane (the runtime must be stopped).", true, clearInterlock),
	)
	return cmd
}

func newInterlockOperationCommand(name, short string, needsReason bool, operation interlockOperation) *cobra.Command {
	var flags operatorFlags
	var reason string
	cmd := &cobra.Command{
		Use: name, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runInterlock(cmd, flags, operation, reason) },
	}
	flags.register(cmd)
	if needsReason {
		cmd.Flags().StringVar(&reason, "reason", "", "Why the interlock changes (required)")
	}
	return cmd
}

func readInterlock(ctx context.Context, db *storage.DB, _ string) (runtimecontrol.InterlockState, error) {
	return runtimecontrol.ReadInterlock(ctx, db) //nolint:wrapcheck // The control module names the failed step.
}

// clearInterlock reopens the action plane while holding the runtime owner
// lease, so it cannot race a running runtime.
func clearInterlock(ctx context.Context, db *storage.DB, reason string) (runtimecontrol.InterlockState, error) {
	var state runtimecontrol.InterlockState
	err := withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) error {
		var clearErr error
		state, clearErr = ownership.owner.ClearInterlock(ctx, ownership.epoch, reason)
		if clearErr != nil {
			return fmt.Errorf("clear interlock: %w", clearErr)
		}
		return nil
	})
	return state, err
}

// runInterlock runs operation and prints the resulting interlock.
func runInterlock(cmd *cobra.Command, flags operatorFlags, operation interlockOperation, reason string) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	state, err := operation(cmd.Context(), db, reason)
	if err != nil {
		return fmt.Errorf("interlock: %w", err)
	}
	return printResult(cmd, flags.asJSON, state, func() string {
		return fmt.Sprintf("interlock: %s (version %d, %s) %s", state.Status, state.Version, state.UpdatedAt, state.Reason)
	})
}
