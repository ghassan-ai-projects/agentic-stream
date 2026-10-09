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

var errDryRun = errors.New("dry run")

func newPrincipalsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "principals", Short: "Provision the approval relays, approvers, roles and authorities."}
	cmd.AddCommand(newPrincipalsApplyCommand(), newPrincipalsShowCommand())
	return cmd
}

func newPrincipalsApplyCommand() *cobra.Command {
	var file string
	var dryRun bool
	return newOperatorCommand("apply --file <principals.yaml>",
		"Make the tenant's approval governance match a principal document (the runtime must be stopped).", cobra.NoArgs,
		func(cmd *cobra.Command, flags operatorFlags, _ []string) error {
			return runPrincipalsApply(cmd, flags, file, dryRun)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&file, "file", "", "Principal document (YAML)")
			cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report the result without changing anything")
		})
}

func newPrincipalsShowCommand() *cobra.Command {
	return newListCommand("show", "Count the tenant's approval governance.", runPrincipalsShow)
}

func runPrincipalsApply(cmd *cobra.Command, flags operatorFlags, file string, dryRun bool) error {
	document, err := readPrincipalDocument(file)
	if err != nil {
		return err
	}
	var summary policy.PrincipalSummary
	if err := withOperatorDatabase(cmd, flags, func(db *storage.DB) (err error) {
		summary, err = applyPrincipalDocument(cmd.Context(), db, document, dryRun)
		return err
	}); err != nil {
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

func applyPrincipalDocument(ctx context.Context, db *storage.DB, document policy.PrincipalDocument, dryRun bool) (policy.PrincipalSummary, error) {
	var summary policy.PrincipalSummary
	err := withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) error {
		return db.WithTx(ctx, func(tx *sql.Tx) (err error) {
			summary, err = applyPrincipals(ctx, tx, ownership, document)
			if err == nil && dryRun {
				return errDryRun
			}
			return err
		})
	})
	if errors.Is(err, errDryRun) {
		return summary, nil
	}
	return summary, err
}

func applyPrincipals(ctx context.Context, tx *sql.Tx, ownership operatorOwnership, document policy.PrincipalDocument) (policy.PrincipalSummary, error) {
	summary, err := policy.ApplyPrincipals(ctx, tx, policy.Ownership{Check: ownership.owner.Assert, Epoch: ownership.epoch}, document, time.Now().UTC())
	if err != nil {
		return policy.PrincipalSummary{}, fmt.Errorf("apply principals: %w", err)
	}
	return summary, nil
}

func runPrincipalsShow(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
	summary, err := governanceSummary(cmd.Context(), db, flags.tenantID)
	if err != nil {
		return err
	}
	return printPrincipalSummary(cmd, flags, summary, false)
}

func governanceSummary(ctx context.Context, db *storage.DB, tenantID string) (policy.PrincipalSummary, error) {
	var summary policy.PrincipalSummary
	err := db.WithTx(ctx, func(tx *sql.Tx) (err error) {
		summary, err = policy.GovernanceSummary(ctx, tx, tenantID)
		if err != nil {
			return fmt.Errorf("summarize governance: %w", err)
		}
		return nil
	})
	if err != nil {
		return policy.PrincipalSummary{}, fmt.Errorf("read governance: %w", err)
	}
	return summary, nil
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
