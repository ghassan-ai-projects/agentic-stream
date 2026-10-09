package transport

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCancellationAndDeadlineStatusesMatchTheContextErrorsAndKeepTheirStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		cause error
		code  codes.Code
	}{
		{name: "canceled", err: status.Error(codes.Canceled, "stop"), cause: context.Canceled, code: codes.Canceled},
		{name: "deadline", err: status.Error(codes.DeadlineExceeded, "slow"), cause: context.DeadlineExceeded, code: codes.DeadlineExceeded},
		{name: "wrapped deadline", err: fmt.Errorf("receive worker event: %w", status.Error(codes.DeadlineExceeded, "slow")), cause: context.DeadlineExceeded, code: codes.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := AsContextError(tt.err)
			if !errors.Is(got, tt.cause) || status.Code(got) != tt.code || got.Error() != tt.err.Error() {
				t.Fatalf("AsContextError(%v) = %v: is cause %v, code %v", tt.err, got, errors.Is(got, tt.cause), status.Code(got))
			}
		})
	}
}

func TestOtherErrorsPassThroughUnchanged(t *testing.T) {
	t.Parallel()

	for _, err := range []error{nil, errors.New("boom"), status.Error(codes.Unavailable, "down")} {
		if got := AsContextError(err); got != err { //nolint:errorlint // Identity is the property under test.
			t.Fatalf("AsContextError(%v) = %v, want the same error", err, got)
		}
		if errors.Is(AsContextError(err), context.Canceled) {
			t.Fatalf("%v classified as cancellation", err)
		}
	}
}
