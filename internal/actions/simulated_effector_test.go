package actions_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
)

func TestSimulatedEffectorAcceptsAnyRouteAndIsIdempotent(t *testing.T) {
	tests := []struct {
		name  string
		route string
	}{
		{name: "maintenance route", route: "create_maintenance_ticket"},
		{name: "domain route", route: "start_aerator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effector := actions.NewSimulatedEffector()
			command := actions.Command{EffectorRoute: tt.route, NormalizedTarget: "pump-1", IdempotencyKey: "sha256:key-" + tt.route}
			first, err := effector.Dispatch(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			second, err := effector.Dispatch(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("duplicate dispatch changed effect: first=%v second=%v", first, second)
			}
			if first.ProviderResult["accepted"] != true || first.ProviderResult["idempotency_key"] != command.IdempotencyKey || first.ObservedEffect["route"] != tt.route {
				t.Fatalf("unexpected simulated effect: %v", first)
			}
		})
	}
}
