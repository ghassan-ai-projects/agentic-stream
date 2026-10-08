package domain

import (
	"context"
	"time"
)

const PersistGrace = 5 * time.Second

func DetachedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), PersistGrace)
}
