package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEveryOwnerAssertionGoesThroughTheOneRuntimeOwnerCheck(t *testing.T) {
	t.Parallel()
	lost := errors.New("owner lost")
	ceiling := uint64(1)
	tests := map[string]struct {
		owner   func(context.Context, *sql.Tx, string) error
		wantErr error
	}{
		"owner holds": {owner: func(context.Context, *sql.Tx, string) error { return nil }},
		"owner lost":  {owner: func(context.Context, *sql.Tx, string) error { return lost }, wantErr: lost},
		"no check":    {wantErr: errOwnerCheckMissing},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			pipeline := &PipelineStore{DB: db, RuntimeOwner: tt.owner, OwnerEpoch: "epoch"}
			admission := pipeline.InAdmission(t.Context(), func(tx *AdmissionTx) error { return tx.AssertOwner(t.Context()) })
			cost := ConfigureCostLimits(t.Context(), CostConfiguration{DB: db, RuntimeOwner: tt.owner, OwnerEpoch: "epoch", Clock: sources.Physical(), TenantID: "tenant", Ceilings: control.CostCeilings{Global: &ceiling}})
			for step, err := range map[string]error{"pipeline": pipeline.AssertOwner(t.Context()), "admission": admission, "cost configuration": cost} {
				if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("%s assertion = %v, want %v", step, err, tt.wantErr)
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
