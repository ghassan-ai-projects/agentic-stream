package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const ownerEpoch = "epoch-1"

type deviceAuthority struct {
	db      *storage.DB
	owner   *control.RuntimeOwner
	service *authority.Service
}

func newDeviceAuthority(t *testing.T) deviceAuthority {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := owner.Claim(t.Context(), ownerEpoch); err != nil {
		t.Fatalf("claim the runtime owner: %v", err)
	}
	service, err := authority.New(authority.Config{DB: db, Owner: owner, Epochs: &control.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatalf("new device authority: %v", err)
	}
	return deviceAuthority{db: db, owner: owner, service: service}
}

func commandingConfig(t *testing.T, devices deviceAuthority) app.Config {
	t.Helper()
	catalog, err := device.LoadCapabilityCatalog([]byte(pumpCatalog))
	if err != nil {
		t.Fatal(err)
	}
	return app.Config{Catalog: catalog, Authority: devices.service, OwnerEpoch: ownerEpoch}
}

func (d deviceAuthority) count(t *testing.T, query string) int {
	t.Helper()
	var count int
	if err := d.db.QueryRowContext(t.Context(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (d deviceAuthority) pumpClaim() authority.TargetClaim {
	return authority.TargetClaim{Target: "P1", Device: authority.DeviceBoot{DeviceID: "P1", BootID: firstBoot},
		Owner: authority.Owner{Epoch: ownerEpoch, Instance: "instance-1"}}
}
