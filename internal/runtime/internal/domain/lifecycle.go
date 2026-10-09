package domain

import (
	"context"
	"errors"
	"time"
)

func OwnerHeartbeatInterval(lease time.Duration) time.Duration {
	interval := lease / 3
	if interval <= 0 {
		return time.Second
	}
	return interval
}

func MaintenanceInterval(configured time.Duration) time.Duration {
	if configured <= 0 {
		return time.Second
	}
	return configured
}

func NormalLiveSocketShutdown(parentErr, err error) bool {
	return parentErr != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

type RouteKind uint8

const (
	WatchRoute RouteKind = iota
	DeviceRoute
	FallbackRoute
)

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

func LeaseLapsed(lastRenewed, now time.Time, lease time.Duration) bool {
	return now.Sub(lastRenewed) >= lease
}
