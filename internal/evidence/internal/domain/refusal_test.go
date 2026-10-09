package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestContextRefusalClassifiesCancellationAndDeadlines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"canceled", context.Canceled, Canceled},
		{"wrapped cancellation", fmt.Errorf("query: %w", context.Canceled), Canceled},
		{"deadline", context.DeadlineExceeded, DeadlineExceeded},
		{"other cause", errors.New("anything"), DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requireRefusal(t, ContextRefusal(test.err), test.want)
		})
	}
}

func TestRefusalMessageIsTheSafePublicText(t *testing.T) {
	t.Parallel()
	err := Refuse(Internal, "evidence query failed")
	if err.Error() != "evidence query failed" {
		t.Fatalf("Error() = %q", err.Error())
	}
}
