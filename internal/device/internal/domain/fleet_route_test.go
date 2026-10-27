package domain_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

const fleetCatalog = `{"protocol_version": 1, "routes": {"service": {
	"operation": "service", "selector_field": "order", "expires_after_ms": 1000, "target_arg": "unit_id",
	"target_bindings": {"unit-1": "dev-1", "unit-2": "dev-2"},
	"presets": {"wear": {"order": "wear"}, "drift": {"order": "drift"}}}}}`

func TestAFleetRouteCommandsTheDeviceBoundToTheEntity(t *testing.T) {
	t.Parallel()
	catalog, err := domain.LoadCapabilityCatalog([]byte(fleetCatalog))
	if err != nil {
		t.Fatalf("load fleet catalog: %v", err)
	}
	if targets := catalog.Routes["service"].Targets(); len(targets) != 2 || targets[0] != "dev-1" || targets[1] != "dev-2" {
		t.Fatalf("targets = %v, want dev-1 and dev-2", targets)
	}
	bounded, err := catalog.Bound(request("service", "unit-2", map[string]any{"order": "wear", "unit_id": "dev-1"}))
	if err != nil {
		t.Fatalf("bound: %v", err)
	}
	if bounded.Target != "dev-2" || bounded.Parameters["unit_id"] != "dev-2" || bounded.Parameters["order"] != "wear" {
		t.Fatalf("bounded = %+v, want dev-2 with its own unit_id and the wear preset", bounded)
	}
	_, err = catalog.Bound(request("service", "unit-3", map[string]any{"order": "wear"}))
	assertRefusal(t, err, "is not bound to catalog target")
	_, err = catalog.Bound(request("service", "dev-1", map[string]any{"order": "wear"}))
	assertRefusal(t, err, "is not bound to catalog target")
}
