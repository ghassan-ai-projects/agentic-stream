package authority_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var (
	ownerOne = authority.Owner{Epoch: "epoch-1", Instance: "instance-1"}
	bootA    = authority.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-A"}
	bootB    = authority.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-B"}
	fanClaim = authority.TargetClaim{Target: "fan-01", Device: bootA, Owner: ownerOne}
)

// fixture is one admitted runtime owner with its device-authority service.
type fixture struct {
	db      *storage.DB
	clock   *clock.Virtual
	runtime *control.RuntimeOwner
	service *authority.Service
}

// runtimeLease is how long ownerOne stays admitted without renewal; target
// claims last ten minutes.
const runtimeLease = time.Hour

// newFixture opens a migrated database, admits ownerOne for runtimeLease, and
// builds its service.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	f := &fixture{db: db, clock: clock.NewVirtual(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))}
	f.runtime, f.service = f.admit(t, ownerOne, runtimeLease)
	return f
}

// admit claims the runtime lease for owner and returns it with its service.
func (f *fixture) admit(t *testing.T, owner authority.Owner, lease time.Duration) (*control.RuntimeOwner, *authority.Service) {
	t.Helper()
	runtimeOwner := &control.RuntimeOwner{DB: f.db, InstanceID: owner.Instance, Lease: lease, Now: f.clock.Now}
	if err := runtimeOwner.Claim(t.Context(), owner.Epoch); err != nil {
		t.Fatalf("claim runtime owner: %v", err)
	}
	service, err := authority.New(authority.Config{
		DB: f.db, Owner: runtimeOwner, Epochs: &control.EpochControl{DB: f.db},
		ClaimLease: 10 * time.Minute, Clock: f.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtimeOwner, service
}

// recordState records a device state for device and returns the document.
func (f *fixture) recordState(t *testing.T, device authority.DeviceBoot) (map[string]any, bool) {
	t.Helper()
	state := map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID, "safe_state": true}
	required, err := f.service.RecordDeviceState(t.Context(), ownerOne, state)
	if err != nil {
		t.Fatalf("record device state: %v", err)
	}
	return state, required
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := f.db.QueryRowContext(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
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
