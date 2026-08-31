package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestNormalLiveSocketShutdownRequiresParentContextTermination(t *testing.T) {
	cases := []struct {
		name      string
		makeCtx   func() (context.Context, context.CancelFunc)
		err       error
		normalEnd bool
	}{
		{
			name:      "active parent deadline is an error",
			makeCtx:   func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			err:       context.DeadlineExceeded,
			normalEnd: false,
		},
		{
			name: "canceled parent is normal",
			makeCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			err:       context.Canceled,
			normalEnd: true,
		},
		{
			name:      "active parent cancellation error is not shutdown",
			makeCtx:   func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			err:       context.Canceled,
			normalEnd: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.makeCtx()
			defer cancel()
			if got := normalLiveSocketShutdown(ctx, tc.err); got != tc.normalEnd {
				t.Fatalf("normalLiveSocketShutdown = %v, want %v", got, tc.normalEnd)
			}
		})
	}
	if normalLiveSocketShutdown(context.Background(), errors.New("other failure")) {
		t.Fatal("non-context error was classified as normal shutdown")
	}
}
