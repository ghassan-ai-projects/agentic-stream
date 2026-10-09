package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
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
	cmd.Flags().StringVar(&f.tenantID, "tenant", contractsv1.TenantID, "Tenant ID")
	cmd.Flags().BoolVar(&f.asJSON, "json", false, "Emit JSON instead of text")
}

func (f *operatorFlags) openOperatorDatabase(ctx context.Context) (*storage.DB, error) {
	if f.dbPath == "" {
		return nil, fmt.Errorf("--db is required")
	}
	db, err := storage.OpenExisting(ctx, f.dbPath)
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

func newOperatorCommand(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, operatorFlags, []string) error, extra ...func(*cobra.Command)) *cobra.Command {
	var flags operatorFlags
	cmd := &cobra.Command{
		Use: use, Short: short, Args: args,
		RunE: func(cmd *cobra.Command, argv []string) error { return run(cmd, flags, argv) },
	}
	flags.register(cmd)
	for _, add := range extra {
		add(cmd)
	}
	return cmd
}

func newDatabaseCommand(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, operatorFlags, *storage.DB, []string) error, extra ...func(*cobra.Command)) *cobra.Command {
	return newOperatorCommand(use, short, args, func(cmd *cobra.Command, flags operatorFlags, argv []string) error {
		return withOperatorDatabase(cmd, flags, func(db *storage.DB) error { return run(cmd, flags, db, argv) })
	}, extra...)
}

func withOperatorDatabase(cmd *cobra.Command, flags operatorFlags, use func(*storage.DB) error) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return use(db)
}

func newListCommand(use, short string, run func(*cobra.Command, operatorFlags, *storage.DB) error, extra ...func(*cobra.Command)) *cobra.Command {
	return newDatabaseCommand(use, short, cobra.NoArgs,
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB, _ []string) error {
			return run(cmd, flags, db)
		},
		extra...)
}

func tableText[T any](rows []T, empty string, line func(T) string) string {
	if len(rows) == 0 {
		return empty
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, line(row))
	}
	return strings.Join(lines, "\n")
}

func newByIDCommand(use, short string, show func(*cobra.Command, operatorFlags, *storage.DB, string) error) *cobra.Command {
	return newDatabaseCommand(use, short, cobra.ExactArgs(1),
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB, args []string) error {
			return show(cmd, flags, db, args[0])
		})
}

func newShowGroup(name, groupShort, idName, showShort string, show func(*cobra.Command, operatorFlags, *storage.DB, string) error) *cobra.Command {
	group := &cobra.Command{Use: name, Short: groupShort}
	group.AddCommand(newByIDCommand("show <"+idName+">", showShort, show))
	return group
}

func newVersionedCommand(use, short string, show func(*cobra.Command, operatorFlags, *storage.DB, string, int) error) *cobra.Command {
	var version int
	return newDatabaseCommand(use, short, cobra.ExactArgs(1),
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB, args []string) error {
			return show(cmd, flags, db, args[0], version)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().IntVar(&version, "version", 0, "Situation version (default: current)")
		})
}
