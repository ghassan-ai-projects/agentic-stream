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

func LeaseLapsed(lastRenewed, now time.Time, lease time.Duration) bool {
	return now.Sub(lastRenewed) >= lease
}
