package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestCompositeEffectorRoutesWatchBeforeSimulatedFallback(t *testing.T) {
	db := storagetest.OpenTemp(t)

	effector := app.NewCompositeEffector(newWatch(t, db), device.NewSimulatedEffector())
	watchCommand := actionport.Command{
		CommandID:     "cmd-watch",
		TenantID:      "tenant-1",
		EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression":        "features.temperature > 90",
			"target":            "motor-1",
			"expires_at":        "2099-01-01T00:00:00Z",
			"situation_id":      "sit-1",
			"situation_version": 1,
			"max_fires":         1,
		},
	}
	watchEffect, err := effector.Dispatch(t.Context(), watchCommand)
	if err != nil {
		t.Fatalf("dispatch watch command: %v", err)
	}
	if watchEffect.ProviderResult["watch_id"] != "cmd-watch" {
		t.Fatalf("watch route used the wrong effector: %v", watchEffect)
	}

	var watchCount int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM watch_conditions WHERE watch_id = ?", "cmd-watch").Scan(&watchCount); err != nil {
		t.Fatal(err)
	}
	if watchCount != 1 {
		t.Fatalf("watch condition count = %d, want 1", watchCount)
	}

	simulatedEffect, err := effector.Dispatch(t.Context(), actionport.Command{
		EffectorRoute:    "start_aerator",
		NormalizedTarget: "pond-1",
		IdempotencyKey:   "sha256:aerator",
	})
	if err != nil {
		t.Fatalf("dispatch simulated fallback command: %v", err)
	}
	if simulatedEffect.ProviderResult["accepted"] != true || simulatedEffect.ObservedEffect["route"] != "start_aerator" {
		t.Fatalf("unexpected simulated fallback effect: %v", simulatedEffect)
	}
}

func TestFailClosedEffectorRejectsUnmappedLiveRoute(t *testing.T) {
	effector := app.NewCompositeEffector(nil, device.NewFailClosedEffector(device.EffectProfilePhysical))
	if _, err := effector.Dispatch(t.Context(), actionport.Command{EffectorRoute: "start_aerator"}); err == nil {
		t.Fatal("physical fallback accepted an unmapped route")
	}
}
