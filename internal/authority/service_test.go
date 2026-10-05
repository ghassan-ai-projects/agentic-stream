package authority_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func openDB(t *testing.T, name string) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return db
}

func TestNewRefusesConfigurationThatSkipsSafetyChecks(t *testing.T) {
	t.Parallel()
	db, other := openDB(t, "runtime.db"), openDB(t, "other.db")
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1"}
	epochs := &control.EpochControl{DB: db}
	tests := []struct {
		name   string
		config authority.Config
		want   string
	}{
		{"missing database", authority.Config{Owner: owner, Epochs: epochs}, "requires a database"},
		{"missing runtime owner", authority.Config{DB: db, Epochs: epochs}, "requires a database"},
		{"missing epoch control", authority.Config{DB: db, Owner: owner}, "requires a database"},
		{"owner on another database", authority.Config{DB: db, Owner: &control.RuntimeOwner{DB: other, InstanceID: "i"}, Epochs: epochs}, "share one database"},
		{"epochs on another database", authority.Config{DB: db, Owner: owner, Epochs: &control.EpochControl{DB: other}}, "share one database"},
		{"owner without instance", authority.Config{DB: db, Owner: &control.RuntimeOwner{DB: db}, Epochs: epochs}, "runtime owner instance"},
		{"negative lease", authority.Config{DB: db, Owner: owner, Epochs: epochs, ClaimLease: -time.Second}, "negative"},
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
	db := openDB(t, "runtime.db")
	runtimeOwner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := runtimeOwner.Claim(ctx, "epoch-1"); err != nil {
		t.Fatal(err)
	}
	service, err := authority.New(authority.Config{DB: db, Owner: runtimeOwner, Epochs: &control.EpochControl{DB: db}})
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
	must(t, service.OpenReconciliation(ctx, device, owner, "untrusted receipt"))
	must(t, service.OpenReconciliationAfterAuthorityLoss(ctx, device, owner, "unknown outcome"))
	required, err := service.ReconciliationRequired(ctx, device.DeviceID)
	must(t, err)
	evidence := evidenceFor(t, state, claim.Target)
	cleared, err := service.ResolveReconciliation(ctx, device, owner, authority.ResolutionSucceeded, evidence)
	must(t, err)
	must(t, db.WithTx(ctx, func(tx *sql.Tx) error {
		return authority.VerifyCommandEvidence(ctx, tx, "cmd-1", claim.Target, evidence)
	}))
	must(t, service.RecordSafeStop(ctx, claim, authority.SafeStopRequested, nil))
	latched, err := service.SafeStopLatched(ctx, device)
	must(t, err)
	must(t, service.RecordSafetyEvent(ctx, authority.SafetyEvent{Type: "unsafe_output", Target: claim.Target}))
	must(t, service.ReleaseClaim(ctx, claim))
	if !required || !cleared || !latched {
		t.Fatalf("required=%v cleared=%v latched=%v", required, cleared, latched)
	}
	if err := service.AssertClaim(ctx, claim); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("released claim = %v", err)
	}
	if err := authority.ValidateReconciliationEvidence(evidence, device); err != nil || !authority.PhysicalEvidenceComplete(map[string]any{
		"evidence_complete": true, "source": "s", "evidence_digest": evidence["state_digest"],
	}) {
		t.Fatalf("stateless rules: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// evidenceFor builds digest-consistent reconciliation evidence for a state.
func evidenceFor(t *testing.T, state map[string]any, target string) map[string]any {
	t.Helper()
	feedback := map[string]any{"target": target, "observed_state": "safe"}
	evidence := map[string]any{
		"device_id": state["device_id"], "boot_id": state["boot_id"], "target": target,
		"evidence_type": "device_state_feedback", "source": "independent-feedback",
		"state": state, "state_digest": digest(t, state),
		"feedback": feedback, "feedback_digest": digest(t, feedback),
	}
	evidence["evidence_digest"] = digest(t, evidence)
	return evidence
}

func digest(t *testing.T, value any) string {
	t.Helper()
	data, err := canonicaljson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return canonicaljson.ContentDigest(data)
}
