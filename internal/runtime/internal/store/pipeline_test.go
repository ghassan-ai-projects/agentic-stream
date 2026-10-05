package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPipelineStorePreservesOriginalFenceAndOperationErrors(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	sentinel := errors.New("owner lost")
	service, err := policy.New(policy.Config{PolicyVersion: "test", Interlock: interlock.DurableReader{}, RuntimeOwner: func(context.Context, *sql.Tx, string) error { return sentinel }, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &PipelineStore{DB: db, Policy: service}
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
	if _, _, err := adapter.NextPendingIntent(t.Context(), "tenant"); err == nil {
		t.Fatal("closed database accepted")
	}
}

func TestCostConfigurationCommitsUnderOwnerFence(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	clk := clock.NewVirtual(now)
	owner := &control.RuntimeOwner{DB: db, InstanceID: "owner", Lease: time.Minute, Now: clk.Now}
	if err := owner.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	adapter := &PipelineStore{DB: db, Owner: owner, OwnerEpoch: "epoch"}
	if err := adapter.AssertOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	ceiling := uint64(100)
	cfg := CostConfiguration{DB: db, Owner: owner, OwnerEpoch: "epoch", Clock: clk, TenantID: "tenant"}
	if err := ConfigureCostLimits(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Ceilings = costcontrol.Ceilings{Global: &ceiling}
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
