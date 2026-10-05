package domain

import (
	"context"
	"errors"
	"time"
)

// OwnerHeartbeatInterval preserves renewal cadence for configured/default leases.
func OwnerHeartbeatInterval(lease time.Duration) time.Duration {
	interval := lease / 3
	if interval <= 0 {
		return time.Second
	}
	return interval
}

// NormalLiveSocketShutdown requires the source parent to be terminated.
func NormalLiveSocketShutdown(parentErr, err error) bool {
	return parentErr != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

// RouteKind identifies the closed route owner, independent of implementation.
type RouteKind uint8

const (
	WatchRoute RouteKind = iota
	DeviceRoute
	FallbackRoute
)

// ClassifyRoute reserves watch and device routes before fallback.
func ClassifyRoute(route string) RouteKind {
	switch route {
	case "install_watch_condition":
		return WatchRoute
	case "set_indicator", "select_thermal_mode":
		return DeviceRoute
	default:
		return FallbackRoute
	}
}
