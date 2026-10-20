package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEveryRouteMustHaveAnOwnerBeforeThePipelineStarts(t *testing.T) {
	t.Parallel()
	watch := app.NewWatch(t, storagetest.OpenTemp(t))
	thermal := []string{"install_watch_condition", "set_indicator", "select_thermal_mode"}
	tests := []struct {
		name     string
		effector *app.CompositeEffector
		routes   []string
		want     string
	}{
		{"a device owns only the routes its catalog names", app.NewCompositeEffector(watch, device.NewSimulatedEffector()).WithDevice(ownerOf("set_indicator")), thermal, `route "select_thermal_mode" has no effector: it is not the watch route and the device catalog does not name it`},
		{"a device that names every route leaves nothing unowned", app.NewCompositeEffector(watch, device.NewFailClosedEffector(device.EffectProfileEmulator)).WithDevice(ownerOf("set_indicator", "select_thermal_mode")), thermal, ""},
		{"without a device the simulated fallback owns every route", app.NewCompositeEffector(watch, device.NewSimulatedEffector()), thermal, ""},
		{"without a device a fallback that declares no routes owns every route", app.NewCompositeEffector(watch, plainEffector{}), []string{"vent_open"}, ""},
		{"without a device the fail-closed fallback owns no route", app.NewCompositeEffector(watch, device.NewFailClosedEffector(device.EffectProfilePhysical)), thermal, `route "set_indicator" has no effector: it is not the watch route and the fallback effector refuses it`},
		{"no fallback owns nothing", app.NewCompositeEffector(watch, nil), []string{"vent_open"}, `route "vent_open" has no effector`},
		{"the watch route needs the watch effector", app.NewCompositeEffector(nil, device.NewSimulatedEffector()), thermal, `route "install_watch_condition" has no effector: the watch effector is not configured`},
		{"no effector at all", nil, thermal, "composite effector is not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.effector.RequireOwners(tt.routes)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("RequireOwners = %v, want %q", err, tt.want)
			}
		})
	}
}
