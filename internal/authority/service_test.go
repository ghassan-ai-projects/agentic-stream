package authority_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNewRefusesConfigurationThatSkipsSafetyChecks(t *testing.T) {
	t.Parallel()
	db, other := storagetest.OpenTemp(t), storagetest.OpenTemp(t)
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1"}
	epochs := &control.EpochControl{DB: db}
	ledger := authority.OutcomeLedger(actions.CountUnresolvedOutcomes)
	tests := []struct {
		name   string
		config authority.Config
		want   string
	}{
		{"missing database", authority.Config{Owner: owner, Epochs: epochs, Outcomes: ledger}, "requires a database"},
		{"missing runtime owner", authority.Config{DB: db, Epochs: epochs, Outcomes: ledger}, "requires a database"},
		{"missing epoch control", authority.Config{DB: db, Owner: owner, Outcomes: ledger}, "requires a database"},
		{"missing outcome ledger", authority.Config{DB: db, Owner: owner, Epochs: epochs}, "outcome ledger"},
		{"owner on another database", authority.Config{DB: db, Owner: &control.RuntimeOwner{DB: other, InstanceID: "i"}, Epochs: epochs, Outcomes: ledger}, "share one database"},
		{"epochs on another database", authority.Config{DB: db, Owner: owner, Epochs: &control.EpochControl{DB: other}, Outcomes: ledger}, "share one database"},
		{"owner without instance", authority.Config{DB: db, Owner: &control.RuntimeOwner{DB: db}, Epochs: epochs, Outcomes: ledger}, "runtime owner instance"},
		{"negative lease", authority.Config{DB: db, Owner: owner, Epochs: epochs, Outcomes: ledger, ClaimLease: -time.Second}, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := authority.New(tt.config); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestServiceDelegatesEveryOperation drives each public operation once
// through the facade; the use cases themselves are tested in internal/app.
func TestServiceDelegatesEveryOperation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)
	runtimeOwner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := runtimeOwner.Claim(ctx, "epoch-1"); err != nil {
		t.Fatal(err)
	}
	service, err := authority.New(authority.Config{DB: db, Owner: runtimeOwner, Epochs: &control.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatal(err)
	}
	owner := authority.Owner{Epoch: "epoch-1", Instance: service.OwnerInstance()}
	device := authority.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-A"}
	claim := authority.TargetClaim{Target: "fan-01", Device: device, Owner: owner}
	state := map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID}
	must(t, service.AssertRuntime(ctx, owner.Epoch))
	must(t, service.Claim(ctx, claim))
	must(t, service.AssertClaim(ctx, claim))
	must(t, service.BindCommand(ctx, authority.CommandBinding{CommandID: "cmd-1", Target: claim.Target, Device: device, Owner: owner}))
	_, err = service.RecordDeviceState(ctx, owner, state)
	must(t, err)
	must(t, service.OpenReconciliation(ctx, authority.ReconciliationOpening{Device: device, Owner: owner, Reason: "untrusted receipt"}))
	must(t, service.OpenReconciliationAfterAuthorityLoss(ctx, authority.ReconciliationOpening{Device: device, Owner: owner, Reason: "unknown outcome"}))
	required, err := service.ReconciliationRequired(ctx, device.DeviceID)
	must(t, err)
	sealed, err := authority.SealReconciliationEvidence("independent-feedback", claim.Target, state, map[string]any{"observed_state": "safe"})
	must(t, err)
	evidence := sealed.Document()
	cleared, err := service.ResolveReconciliation(ctx, authority.ResolutionRequest{Device: device, Owner: owner, Outcome: authority.ResolutionSucceeded, Evidence: evidence})
	must(t, err)
	must(t, db.WithTx(ctx, func(tx *sql.Tx) error {
		return authority.VerifyCommandEvidence(ctx, tx, authority.CommandEvidence{CommandID: "cmd-1", Target: claim.Target, Evidence: evidence})
	}))
	must(t, service.RecordSafeStop(ctx, claim, authority.SafeStopRequested, nil))
	latched, err := service.SafeStopLatched(ctx, device)
	must(t, err)
	must(t, service.RecordSafetyEvent(ctx, authority.SafetyEvent{Type: authority.SafetyUnsafeOutput, Target: claim.Target}))
	var record authority.SafetyRecord
	must(t, db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		record, err = authority.ReadSafetyRecord(ctx, tx)
		return err
	}))
	if record.EventCounts[authority.SafetyUnsafeOutput] != 1 || record.AuthorityEvents == 0 {
		t.Fatalf("safety record = %+v", record)
	}
	must(t, service.ReleaseClaim(ctx, claim))
	if !required || !cleared || !latched {
		t.Fatalf("required=%v cleared=%v latched=%v", required, cleared, latched)
	}
	if err := service.AssertClaim(ctx, claim); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("released claim = %v", err)
	}
}

func TestUnsetClaimLeaseIsTheSourcesDefaultLease(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)
	clock := sources.NewVirtual(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	runtimeOwner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour, Now: clock.Now}
	must(t, runtimeOwner.Claim(ctx, "epoch-1"))
	service, err := authority.New(authority.Config{DB: db, Owner: runtimeOwner, Epochs: &control.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes, Clock: clock})
	must(t, err)
	claim := authority.TargetClaim{Target: "fan-01", Device: authority.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-A"}, Owner: authority.Owner{Epoch: "epoch-1", Instance: service.OwnerInstance()}}
	must(t, service.Claim(ctx, claim))
	clock.Advance(sources.DefaultLease - time.Nanosecond)
	must(t, service.AssertClaim(ctx, claim))
	clock.Advance(time.Nanosecond)
	if err := service.AssertClaim(ctx, claim); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("claim at the default lease = %v, want not owned", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
