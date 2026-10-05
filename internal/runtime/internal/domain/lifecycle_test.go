package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestLeaseRenewalAndShutdownRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ lease, want time.Duration }{{0, time.Second}, {-time.Second, time.Second}, {2, time.Second}, {3, 1}, {9 * time.Second, 3 * time.Second}} {
		if got := OwnerHeartbeatInterval(tc.lease); got != tc.want {
			t.Errorf("lease %s: got %s want %s", tc.lease, got, tc.want)
		}
	}
	for _, tc := range []struct {
		parent, source error
		want           bool
	}{
		{nil, context.Canceled, false}, {nil, context.DeadlineExceeded, false}, {context.Canceled, context.Canceled, true},
		{context.DeadlineExceeded, fmt.Errorf("source: %w", context.DeadlineExceeded), true},
		{context.Canceled, errors.New("source failed"), false}, {context.Canceled, nil, false},
	} {
		if got := NormalLiveSocketShutdown(tc.parent, tc.source); got != tc.want {
			t.Errorf("shutdown(%v,%v) = %v", tc.parent, tc.source, got)
		}
	}
}

func TestClosedRouteOwnership(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		route string
		want  RouteKind
	}{
		{"install_watch_condition", WatchRoute}, {"set_indicator", DeviceRoute}, {"select_thermal_mode", DeviceRoute}, {"start_aerator", FallbackRoute}, {"", FallbackRoute},
	} {
		if got := ClassifyRoute(tc.route); got != tc.want {
			t.Errorf("route %q: got %v want %v", tc.route, got, tc.want)
		}
	}
}

func TestEvidenceKeyRejectsMalformedHex(t *testing.T) {
	t.Parallel()
	if _, err := DecodeEvidenceKey("not-hex"); err == nil {
		t.Fatal("malformed key accepted")
	}
}
