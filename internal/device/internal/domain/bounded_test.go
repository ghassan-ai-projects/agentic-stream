package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestBoundResolvesAnApprovedCommandWithoutADeviceLink(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	command := request("select_thermal_mode", "zone-01", map[string]any{"mode": "bounded_cooling", "duty_permille": float64(999)})
	bounded, err := catalog.Bound(command)
	if err != nil {
		t.Fatalf("Bound = %v", err)
	}
	want := domain.BoundedCommand{Route: "select_thermal_mode", Target: "fan-01", Operation: "set_pwm_lease",
		Parameters: map[string]any{"duty_permille": float64(450), "lease_ms": float64(5000)}, ExpiresAfterMs: 20000}
	if !reflect.DeepEqual(bounded, want) {
		t.Fatalf("Bound = %+v, want %+v", bounded, want)
	}
	document, err := documentOf(catalog.Materialize(command, bootID))
	if err != nil || !reflect.DeepEqual(document["parameters"], bounded.Parameters) || document["target"] != bounded.Target {
		t.Fatalf("Materialize = %v, %v; want the bounded command's target and parameters", document, err)
	}
}

func TestBoundRefusesWhatTheCatalogDoesNotAllow(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	cases := []struct {
		name    string
		catalog *domain.CapabilityCatalog
		route   string
		target  string
		payload map[string]any
		want    string
	}{
		{"no catalog", nil, "set_indicator", "led-01", map[string]any{"state": "alert"}, "capability catalog is required"},
		{"an unknown route", catalog, "open_valve", "led-01", map[string]any{"state": "alert"}, "is not in the device capability catalog"},
		{"an unbound target", catalog, "set_indicator", "zone-02", map[string]any{"state": "alert"}, "is not bound to catalog target"},
		{"a selector outside the presets", catalog, "set_indicator", "led-01", map[string]any{"state": "party"}, "is not an allowed preset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.catalog.Bound(request(tc.route, tc.target, tc.payload))
			assertRefusal(t, err, tc.want)
		})
	}
}

func TestCheckExpiriesWithinRefusesARouteThatOutlivesTheDispatchCall(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	if err := catalog.CheckExpiriesWithin(54 * time.Second); !errors.Is(err, domain.ErrRouteOutlivesDispatch) || !strings.Contains(err.Error(), `"set_indicator"`) {
		t.Fatalf("CheckExpiriesWithin(54s) = %v, want set_indicator (60 s) refused", err)
	}
	if err := catalog.CheckExpiriesWithin(60 * time.Second); !errors.Is(err, domain.ErrRouteOutlivesDispatch) {
		t.Fatalf("CheckExpiriesWithin(60s) = %v, want a route expiring exactly at the call's end refused", err)
	}
	if err := catalog.CheckExpiriesWithin(162 * time.Second); err != nil {
		t.Fatalf("CheckExpiriesWithin(162s) = %v, want every route inside", err)
	}
}
