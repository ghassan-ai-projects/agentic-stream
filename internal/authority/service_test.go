package authority_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestNewRefusesConfigurationThatSkipsSafetyChecks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	other, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	owner := &control.RuntimeOwner{DB: f.db, InstanceID: "instance-1"}
	epochs := &control.EpochControl{DB: f.db}
	tests := []struct {
		name   string
		config authority.Config
		want   string
	}{
		{"missing database", authority.Config{Owner: owner, Epochs: epochs}, "requires a database"},
		{"missing runtime owner", authority.Config{DB: f.db, Epochs: epochs}, "requires a database"},
		{"missing epoch control", authority.Config{DB: f.db, Owner: owner}, "requires a database"},
		{"owner on another database", authority.Config{DB: f.db, Owner: &control.RuntimeOwner{DB: other, InstanceID: "i"}, Epochs: epochs}, "share one database"},
		{"epochs on another database", authority.Config{DB: f.db, Owner: owner, Epochs: &control.EpochControl{DB: other}}, "share one database"},
		{"owner without instance", authority.Config{DB: f.db, Owner: &control.RuntimeOwner{DB: f.db}, Epochs: epochs}, "runtime owner instance"},
		{"negative lease", authority.Config{DB: f.db, Owner: owner, Epochs: epochs, ClaimLease: -time.Second}, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := authority.New(tt.config); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New = %v, want %q", err, tt.want)
			}
		})
	}
	service, err := authority.New(authority.Config{DB: f.db, Owner: owner, Epochs: epochs})
	if err != nil || service.OwnerInstance() != "instance-1" {
		t.Fatalf("defaults: %v", err)
	}
}

func TestOperationsRefuseAnotherOwnerInstance(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	foreign := authority.Owner{Epoch: "epoch-1", Instance: "instance-2"}
	claim := fanClaim
	claim.Owner = foreign
	checks := map[string]error{
		"claim":         f.service.Claim(t.Context(), claim),
		"safe stop":     f.service.RecordSafeStop(t.Context(), claim, authority.SafeStopRequested, nil),
		"open":          f.service.OpenReconciliation(t.Context(), bootA, foreign, "reason"),
		"bind command":  f.service.BindCommand(t.Context(), authority.CommandBinding{CommandID: "c", Target: "fan-01", Device: bootA, Owner: foreign}),
		"partial owner": f.service.Claim(t.Context(), authority.TargetClaim{Target: "fan-01", Device: bootA, Owner: authority.Owner{Instance: "instance-1"}}),
	}
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s accepted a foreign or partial owner", name)
		}
	}
	if _, err := f.service.RecordDeviceState(t.Context(), foreign, map[string]any{"device_id": "d", "boot_id": "b"}); err == nil {
		t.Error("device state accepted a foreign owner")
	}
}

func TestAssertRuntimeFollowsTheRuntimeLease(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.AssertRuntime(t.Context(), ownerOne.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := f.service.AssertRuntime(t.Context(), ""); err == nil {
		t.Fatal("empty epoch was admitted")
	}
	f.clock.Advance(runtimeLease)
	if err := f.service.AssertRuntime(t.Context(), ownerOne.Epoch); !errors.Is(err, control.ErrRuntimeOwnerBusy) {
		t.Fatalf("expired runtime lease = %v", err)
	}
}
