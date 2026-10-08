package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestPipelineStorePreservesOriginalFenceAndOperationErrors(t *testing.T) {
	db := storagetest.OpenTemp(t)

	sentinel := errors.New("owner lost")
	service, err := policy.New(policy.Config{PolicyVersion: "test", RuntimeOwner: func(context.Context, *sql.Tx, string) error { return sentinel }, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &PipelineStore{DB: db, Policy: service, RuntimeOwner: func(context.Context, *sql.Tx, string) error { return nil }}
	if err := adapter.AssertOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, found, err := adapter.NextPendingIntent(t.Context(), "tenant"); err != nil || found {
		t.Fatal(found, err)
	}
	if err := adapter.EvaluateIntent(t.Context(), "missing", time.Now()); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := adapter.ApprovalForSigning(t.Context(), policy.ApprovalLookup{ID: "missing"}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := adapter.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "missing"}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, wantErr := policy.NextPendingIntent(t.Context(), db.DB, "tenant")
	if _, _, err := adapter.NextPendingIntent(t.Context(), "tenant"); err == nil || err.Error() != wantErr.Error() {
		t.Fatalf("pending read error = %v, want original %v", err, wantErr)
	}
}

func TestCostConfigurationCommitsUnderOwnerFence(t *testing.T) {
	db := storagetest.OpenTemp(t)

	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	clk := sources.NewVirtual(now)
	owner := &control.RuntimeOwner{DB: db, InstanceID: "owner", Lease: time.Minute, Now: clk.Now}
	if err := owner.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	adapter := &PipelineStore{DB: db, RuntimeOwner: owner.Assert, OwnerEpoch: "epoch"}
	if err := adapter.AssertOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	ceiling := uint64(100)
	cfg := CostConfiguration{DB: db, RuntimeOwner: owner.Assert, OwnerEpoch: "epoch", Clock: clk, TenantID: "tenant"}
	if err := ConfigureCostLimits(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Ceilings = control.CostCeilings{Global: &ceiling}
	if err := ConfigureCostLimits(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Minute)
	if err := ConfigureCostLimits(t.Context(), cfg); err == nil {
		t.Fatal("expired owner accepted")
	}
	if err := adapter.AssertOwner(t.Context()); err == nil {
		t.Fatal("expired owner accepted")
	}
}

func TestEveryOwnerAssertionGoesThroughTheOneRuntimeOwnerCheck(t *testing.T) {
	t.Parallel()
	lost := errors.New("owner lost")
	ceiling := uint64(1)
	for name, tc := range map[string]struct {
		owner   func(context.Context, *sql.Tx, string) error
		wantErr error
	}{
		"owner holds": {owner: func(context.Context, *sql.Tx, string) error { return nil }},
		"owner lost":  {owner: func(context.Context, *sql.Tx, string) error { return lost }, wantErr: lost},
		"no check":    {wantErr: errOwnerCheckMissing},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			pipeline := &PipelineStore{DB: db, RuntimeOwner: tc.owner, OwnerEpoch: "epoch"}
			admission := pipeline.InAdmission(t.Context(), func(tx *AdmissionTx) error { return tx.AssertOwner(t.Context()) })
			cost := ConfigureCostLimits(t.Context(), CostConfiguration{DB: db, RuntimeOwner: tc.owner, OwnerEpoch: "epoch", Clock: sources.Physical(), TenantID: "tenant", Ceilings: control.CostCeilings{Global: &ceiling}})
			for step, err := range map[string]error{"pipeline": pipeline.AssertOwner(t.Context()), "admission": admission, "cost configuration": cost} {
				if tc.wantErr == nil && err != nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("%s assertion = %v, want %v", step, err, tc.wantErr)
				}
			}
		})
	}
}

func TestOwnerAssertionPassesTheBoundEpoch(t *testing.T) {
	t.Parallel()
	var asked string
	pipeline := &PipelineStore{DB: storagetest.OpenTemp(t), OwnerEpoch: "epoch-7", RuntimeOwner: func(_ context.Context, _ *sql.Tx, epoch string) error {
		asked = epoch
		return nil
	}}
	if err := pipeline.AssertOwner(t.Context()); err != nil || asked != "epoch-7" {
		t.Fatalf("asked %q err=%v", asked, err)
	}
}
