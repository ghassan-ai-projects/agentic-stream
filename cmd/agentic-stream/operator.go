package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const operatorLease = 30 * time.Second

type operatorFlags struct {
	dbPath, tenantID string
	asJSON           bool
}

func (f *operatorFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.dbPath, "db", "", "Runtime SQLite database path")
	cmd.Flags().StringVar(&f.tenantID, "tenant", "default", "Tenant ID")
	cmd.Flags().BoolVar(&f.asJSON, "json", false, "Emit JSON instead of text")
}

func (f *operatorFlags) openOperatorDatabase(ctx context.Context) (*storage.DB, error) {
	if f.dbPath == "" {
		return nil, fmt.Errorf("--db is required")
	}
	db, err := storage.Open(ctx, f.dbPath)
	if err != nil {
		return nil, fmt.Errorf("open runtime database: %w", err)
	}
	return db, nil
}

type operatorOwnership struct {
	owner *runtimecontrol.RuntimeOwner
	epoch string
}

func withRuntimeOwnership(ctx context.Context, db *storage.DB, change func(operatorOwnership) error) error {
	epoch, err := evidence.NewRuntimeEpoch()
	if err != nil {
		return fmt.Errorf("generate operator epoch: %w", err)
	}
	ownership := operatorOwnership{owner: &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "operator-" + epoch, Lease: operatorLease}, epoch: epoch}
	if err := ownership.owner.Claim(ctx, epoch); err != nil {
		return fmt.Errorf("claim runtime ownership (stop the running runtime first): %w", err)
	}
	defer func() { _ = ownership.owner.Release(context.WithoutCancel(ctx), epoch) }()
	return change(ownership)
}

func printResult(cmd *cobra.Command, asJSON bool, value any, format func() string) error {
	if !asJSON {
		cmd.Println(format())
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	cmd.Println(string(encoded))
	return nil
}
