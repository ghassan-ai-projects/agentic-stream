package actions_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
)

func TestSimulatedEffectorIsIdempotentForMaintenanceRoutes(t *testing.T) {
	effector := actions.NewSimulatedEffector()
	command := actions.Command{EffectorRoute: "create_maintenance_ticket", NormalizedTarget: "pump-1", IdempotencyKey: "sha256:key"}
	first, err := effector.Dispatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := effector.Dispatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProviderResult["accepted"] != true || second.ProviderResult["idempotency_key"] != "sha256:key" {
		t.Fatalf("unexpected simulated effects: first=%v second=%v", first, second)
	}
}
