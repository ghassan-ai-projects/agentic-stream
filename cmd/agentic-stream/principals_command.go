package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// errDryRun rolls back a dry-run apply after it has computed its result.
var errDryRun = errors.New("dry run")

func newPrincipalsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "principals", Short: "Provision the approval relays, approvers, roles and authorities."}
	cmd.AddCommand(newPrincipalsApplyCommand(), newPrincipalsShowCommand())
	return cmd
}

func newPrincipalsApplyCommand() *cobra.Command {
	var flags operatorFlags
	var file string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply --file <principals.yaml>",
		Short: "Make the tenant's approval governance match a principal document (the runtime must be stopped).",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runPrincipalsApply(cmd, flags, file, dryRun) },
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&file, "file", "", "Principal document (YAML)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report the result without changing anything")
	return cmd
}

func newPrincipalsShowCommand() *cobra.Command {
	var flags operatorFlags
	cmd := &cobra.Command{
		Use: "show", Short: "Count the tenant's approval governance.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runPrincipalsShow(cmd, flags) },
	}
	flags.register(cmd)
	return cmd
}

func runPrincipalsApply(cmd *cobra.Command, flags operatorFlags, file string, dryRun bool) error {
	document, err := readPrincipalDocument(file)
	if err != nil {
		return err
	}
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	summary, err := applyPrincipalDocument(cmd.Context(), db, document, dryRun)
	if err != nil {
		return err
	}
	return printPrincipalSummary(cmd, flags, summary, dryRun)
}

func readPrincipalDocument(file string) (policy.PrincipalDocument, error) {
	if file == "" {
		return policy.PrincipalDocument{}, fmt.Errorf("--file is required")
	}
	data, err := os.ReadFile(file) //nolint:gosec // The operator names the document to apply.
	if err != nil {
		return policy.PrincipalDocument{}, fmt.Errorf("read principal document: %w", err)
	}
	document, err := policy.ParsePrincipals(data)
	if err != nil {
		return policy.PrincipalDocument{}, fmt.Errorf("principal document %s: %w", file, err)
	}
	return document, nil
}

// applyPrincipalDocument applies the document under the runtime owner lease;
// a dry run rolls the transaction back after computing the result.
func applyPrincipalDocument(ctx context.Context, db *storage.DB, document policy.PrincipalDocument, dryRun bool) (policy.PrincipalSummary, error) {
	var summary policy.PrincipalSummary
	err := withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error {
			var err error
			summary, err = policy.ApplyPrincipals(ctx, tx, policy.Ownership{Check: ownership.owner.Assert, Epoch: ownership.epoch}, document, time.Now().UTC())
			if err == nil && dryRun {
				return errDryRun
			}
			return err //nolint:wrapcheck // The policy module names the failed step.
		})
	})
	if errors.Is(err, errDryRun) {
		return summary, nil
	}
	return summary, err
}

func runPrincipalsShow(cmd *cobra.Command, flags operatorFlags) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var summary policy.PrincipalSummary
	if err := db.WithTx(cmd.Context(), func(tx *sql.Tx) (err error) {
		summary, err = policy.GovernanceSummary(cmd.Context(), tx, flags.tenantID)
		return err //nolint:wrapcheck // The policy module names the failed step.
	}); err != nil {
		return fmt.Errorf("read governance: %w", err)
	}
	return printPrincipalSummary(cmd, flags, summary, false)
}

func printPrincipalSummary(cmd *cobra.Command, flags operatorFlags, summary policy.PrincipalSummary, dryRun bool) error {
	return printResult(cmd, flags.asJSON, summary, func() string {
		prefix := "principals"
		if dryRun {
			prefix = "principals (dry run, nothing changed)"
		}
		return fmt.Sprintf("%s: tenant=%s active=%d disabled=%d roles=%d memberships=%d authorities=%d",
			prefix, summary.Tenant, summary.Active, summary.Disabled, summary.Roles, summary.Memberships, summary.Authorities)
	})
}
