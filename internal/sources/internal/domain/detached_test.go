package domain

import (
	"context"
	"testing"
	"time"
)

type detachedKey struct{}

func TestDetachedContextKeepsValuesDropsCancellationAndIsBounded(t *testing.T) {
	t.Parallel()
	parent, cancelParent := context.WithCancel(context.WithValue(t.Context(), detachedKey{}, "trace"))
	cancelParent()
	detached, cancel := DetachedContext(parent)
	defer cancel()
	if detached.Err() != nil {
		t.Fatalf("detached context inherited cancellation: %v", detached.Err())
	}
	if detached.Value(detachedKey{}) != "trace" {
		t.Fatal("detached context lost the caller's values")
	}
	deadline, ok := detached.Deadline()
	if remaining := time.Until(deadline); !ok || remaining > PersistGrace || remaining <= 0 {
		t.Fatalf("detached context must expire within PersistGrace: deadline=%v ok=%v remaining=%v", deadline, ok, remaining)
	}
}

func TestDetachedContextEndsWhenItsCancelFunctionIsCalled(t *testing.T) {
	t.Parallel()
	detached, cancel := DetachedContext(t.Context())
	cancel()
	if detached.Err() == nil {
		t.Fatal("canceling the returned function must end the detached context")
	}
}
