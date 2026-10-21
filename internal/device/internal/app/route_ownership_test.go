package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestEachEffectorOwnsTheRoutesItsProfileAllows(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	var unconfigured *app.GatewayEffector
	tests := []struct {
		name  string
		owner actionport.RouteOwner
		route string
		want  bool
	}{
		{"the gateway owns a route its catalog names", app.NewGatewayEffector(nil, catalog), "select_thermal_mode", true},
		{"the gateway does not own a route its catalog omits", app.NewGatewayEffector(nil, catalog), "vent_open", false},
		{"a gateway without a catalog owns nothing", app.NewGatewayEffector(nil, nil), "set_indicator", false},
		{"an unconfigured gateway owns nothing", unconfigured, "set_indicator", false},
		{"the simulated effector owns every route", app.NewSimulatedEffector(), "vent_open", true},
		{"the fail-closed effector owns no route", app.NewFailClosedEffector(domain.EffectProfilePhysical), "set_indicator", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.owner.OwnsRoute(tt.route); got != tt.want {
				t.Fatalf("OwnsRoute(%q) = %v, want %v", tt.route, got, tt.want)
			}
		})
	}
}
