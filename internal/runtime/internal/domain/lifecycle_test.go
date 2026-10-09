package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestOwnerHeartbeatRenewsThreeTimesPerLeaseAndAtLeastEverySecondForUnsetLeases(t *testing.T) {
	t.Parallel()
	tests := []struct{ lease, want time.Duration }{
		{0, time.Second}, {-time.Second, time.Second}, {2, time.Second}, {3, 1}, {9 * time.Second, 3 * time.Second},
	}
	for _, tt := range tests {
		if got := OwnerHeartbeatInterval(tt.lease); got != tt.want {
			t.Errorf("lease %s: got %s want %s", tt.lease, got, tt.want)
		}
	}
}

func TestMaintenanceIntervalKeepsTheOneSecondDefaultUntilConfigured(t *testing.T) {
	t.Parallel()
	tests := []struct{ configured, want time.Duration }{
		{0, time.Second}, {-time.Millisecond, time.Second}, {time.Millisecond, time.Millisecond}, {time.Minute, time.Minute},
	}
	for _, tt := range tests {
		if got := MaintenanceInterval(tt.configured); got != tt.want {
			t.Errorf("MaintenanceInterval(%s) = %s, want %s", tt.configured, got, tt.want)
		}
	}
}

func TestALiveSocketEndsNormallyOnlyWhenItsParentWasTerminated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		parent, source error
		want           bool
	}{
		{"source cancellation without a parent cancellation is a failure", nil, context.Canceled, false},
		{"source deadline without a parent deadline is a failure", nil, context.DeadlineExceeded, false},
		{"parent canceled and source canceled", context.Canceled, context.Canceled, true},
		{"parent deadline and wrapped source deadline", context.DeadlineExceeded, fmt.Errorf("source: %w", context.DeadlineExceeded), true},
		{"parent canceled but the source failed on its own", context.Canceled, errors.New("source failed"), false},
		{"parent canceled and the source ended cleanly", context.Canceled, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalLiveSocketShutdown(tt.parent, tt.source); got != tt.want {
				t.Fatalf("NormalLiveSocketShutdown(%v, %v) = %v, want %v", tt.parent, tt.source, got, tt.want)
			}
		})
	}
}

func TestClosedRoutesAreOwnedBeforeTheFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		route string
		want  RouteKind
	}{
		{"install_watch_condition", WatchRoute}, {"set_indicator", DeviceRoute}, {"select_thermal_mode", DeviceRoute},
		{"start_aerator", FallbackRoute}, {"", FallbackRoute}, {"set_indicator ", FallbackRoute},
	}
	for _, tt := range tests {
		if got := ClassifyRoute(tt.route); got != tt.want {
			t.Errorf("route %q: got %v want %v", tt.route, got, tt.want)
		}
	}
}

func TestAFailedRenewalLosesTheLeaseOnlyOnceTheLeaseHasRunOut(t *testing.T) {
	t.Parallel()
	renewed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if LeaseLapsed(renewed, renewed.Add(59*time.Second), time.Minute) {
		t.Fatal("a renewal failure inside the lease lost it")
	}
	if !LeaseLapsed(renewed, renewed.Add(time.Minute), time.Minute) {
		t.Fatal("a lease past its duration is still held")
	}
}
