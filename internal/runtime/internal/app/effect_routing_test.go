package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

type recordingRoute struct {
	dispatches, authorized int
	verifyStatus           string
	verifyErr              error
}

func (r *recordingRoute) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	r.dispatches++
	return actionport.Effect{ProviderResult: map[string]any{"route": "recorded"}}, nil
}

func (r *recordingRoute) DispatchAuthorized(ctx context.Context, command actionport.Command, auth actionport.Authorization) (actionport.Effect, error) {
	if err := auth.Check(ctx); err != nil {
		return actionport.Effect{}, err
	}
	r.authorized++
	return r.Dispatch(ctx, command)
}

func (r *recordingRoute) VerifyDeviceCommand(context.Context, actionport.Command) (string, map[string]any, error) {
	return r.verifyStatus, map[string]any{"verified": true}, r.verifyErr
}

type plainEffector struct{}

func (plainEffector) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	return actionport.Effect{}, nil
}

func installWatchCommand() actionport.Command {
	return actionport.Command{
		CommandID: "cmd-watch", TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression": "features.temperature > 90", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "sit-1", "situation_version": 1, "max_fires": 1,
		},
	}
}

func TestWatchRoutesAreServedByTheWatchEffectorBeforeAnyFallback(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	fallback := &recordingRoute{}
	effector := app.NewCompositeEffector(app.NewWatch(t, db), fallback)

	effect, err := effector.Dispatch(t.Context(), installWatchCommand())
	if err != nil {
		t.Fatalf("dispatch watch command: %v", err)
	}
	if effect.ProviderResult["watch_id"] != "cmd-watch" || fallback.dispatches != 0 {
		t.Fatalf("watch route used the wrong effector: effect=%v fallback dispatches=%d", effect, fallback.dispatches)
	}
	if count := scalar[int](t, db, "SELECT COUNT(*) FROM watch_conditions WHERE watch_id = 'cmd-watch'"); count != 1 {
		t.Fatalf("watch conditions = %d, want 1", count)
	}
}

func TestOtherRoutesGoToTheFallbackAndAPhysicalFallbackFailsClosed(t *testing.T) {
	t.Parallel()
	command := actionport.Command{EffectorRoute: "start_aerator", NormalizedTarget: "pond-1", IdempotencyKey: "sha256:aerator"}
	simulated := app.NewCompositeEffector(nil, device.NewSimulatedEffector())
	effect, err := simulated.Dispatch(t.Context(), command)
	if err != nil || effect.ProviderResult["accepted"] != true || effect.ObservedEffect["route"] != "start_aerator" {
		t.Fatalf("simulated fallback: effect=%v err=%v", effect, err)
	}
	physical := app.NewCompositeEffector(nil, device.NewFailClosedEffector(device.EffectProfilePhysical))
	if _, err := physical.Dispatch(t.Context(), command); err == nil || !strings.Contains(err.Error(), `has no effector for route "start_aerator"`) {
		t.Fatalf("physical fallback = %v, want the unmapped route refused", err)
	}
}

func TestARouteWithoutItsEffectorFailsClosedInsteadOfFallingBack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		effector *app.CompositeEffector
		route    string
		want     string
	}{
		{"no watch effector for a watch route", app.NewCompositeEffector(nil, &recordingRoute{}), "install_watch_condition", "watch effector is not configured"},
		{"no device effector for set_indicator", app.NewCompositeEffector(nil, &recordingRoute{}), "set_indicator", `serial effector is not configured for route "set_indicator"`},
		{"no device effector for select_thermal_mode", app.NewCompositeEffector(nil, &recordingRoute{}), "select_thermal_mode", `serial effector is not configured for route "select_thermal_mode"`},
		{"no fallback for an ordinary route", app.NewCompositeEffector(nil, nil), "start_aerator", `no effector is configured for route "start_aerator"`},
		{"no effector at all", nil, "start_aerator", "composite effector is not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.effector.Dispatch(t.Context(), actionport.Command{EffectorRoute: tt.route}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("route %s = %v, want it refused with %q", tt.route, err, tt.want)
			}
		})
	}
}

func TestDeviceRoutesNeverReachTheFallback(t *testing.T) {
	t.Parallel()
	fallback := &recordingRoute{}
	routes := app.NewCompositeEffector(nil, fallback)
	for _, route := range []string{"set_indicator", "select_thermal_mode"} {
		if _, err := routes.Dispatch(t.Context(), actionport.Command{EffectorRoute: route}); err == nil || !strings.Contains(err.Error(), "serial effector is not configured") {
			t.Fatalf("missing device route %s = %v, want it refused instead of falling through to the fallback", route, err)
		}
	}
	physical := &recordingRoute{}
	routes.WithSerial(physical)
	if _, err := routes.Dispatch(t.Context(), actionport.Command{EffectorRoute: "set_indicator"}); err != nil {
		t.Fatal(err)
	}
	if fallback.dispatches != 0 || physical.dispatches != 1 {
		t.Fatalf("fallback dispatches=%d device dispatches=%d; want 0 and 1", fallback.dispatches, physical.dispatches)
	}
}

func TestAuthorizedDispatchKeepsTheFinalAuthorizationCheck(t *testing.T) {
	t.Parallel()
	denied := errors.New("interlock denied")
	tests := []struct {
		name    string
		route   string
		auth    actionport.Authorization
		wantErr error
		refused bool
		wantRun int
	}{
		{"device route runs after the check passes", "set_indicator", actionport.Authorization{Check: func(context.Context) error { return nil }}, nil, false, 1},
		{"device route is refused when the check denies", "set_indicator", actionport.Authorization{Check: func(context.Context) error { return denied }}, denied, true, 0},
		{"ordinary route is refused when the check denies", "start_aerator", actionport.Authorization{Check: func(context.Context) error { return denied }}, denied, true, 0},
		{"missing authorization never reaches the effect", "set_indicator", actionport.Authorization{}, nil, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fallback, deviceEffector := &recordingRoute{}, &recordingRoute{}
			routes := app.NewCompositeEffector(nil, fallback).WithSerial(deviceEffector)
			_, err := routes.DispatchAuthorized(t.Context(), actionport.Command{EffectorRoute: tt.route}, tt.auth)
			if (err != nil) != tt.refused || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want refused=%v wrapping %v", err, tt.refused, tt.wantErr)
			}
			if got := fallback.authorized + deviceEffector.authorized; got != tt.wantRun {
				t.Fatalf("effects accepted = %d, want %d", got, tt.wantRun)
			}
		})
	}
}

func TestAuthorizedDispatchRequiresAnEffectorThatCanEnforceAuthorization(t *testing.T) {
	t.Parallel()
	routes := app.NewCompositeEffector(nil, plainEffector{})
	auth := actionport.Authorization{Check: func(context.Context) error { return nil }}
	if _, err := routes.DispatchAuthorized(t.Context(), actionport.Command{EffectorRoute: "start_aerator"}, auth); err == nil {
		t.Fatal("an effector that cannot enforce authorization was used")
	}
}

func TestDeviceVerificationIsRoutedOnlyForDeviceCommands(t *testing.T) {
	t.Parallel()
	failure := errors.New("device offline")
	tests := []struct {
		name       string
		effector   *app.CompositeEffector
		route      string
		wantStatus string
		wantErr    error
	}{
		{"device command is verified by the device effector", app.NewCompositeEffector(nil, nil).WithSerial(&recordingRoute{verifyStatus: "succeeded"}), "set_indicator", "succeeded", nil},
		{"a verification failure is returned with its status", app.NewCompositeEffector(nil, nil).WithSerial(&recordingRoute{verifyStatus: "failed", verifyErr: failure}), "select_thermal_mode", "failed", failure},
		{"a non-device command has nothing to verify", app.NewCompositeEffector(nil, nil).WithSerial(&recordingRoute{verifyStatus: "succeeded"}), "start_aerator", "", nil},
		{"a device command without a device effector has nothing to verify", app.NewCompositeEffector(nil, nil), "set_indicator", "", nil},
		{"no effector at all", nil, "set_indicator", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, _, err := tt.effector.VerifyDeviceCommand(t.Context(), actionport.Command{EffectorRoute: tt.route})
			if status != tt.wantStatus || !errors.Is(err, tt.wantErr) {
				t.Fatalf("verification = %q, %v; want %q, %v", status, err, tt.wantStatus, tt.wantErr)
			}
		})
	}
}
