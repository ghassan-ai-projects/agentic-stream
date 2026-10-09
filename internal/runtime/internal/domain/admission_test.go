package domain

import (
	"errors"
	"testing"
)

func TestFixtureExecutorIsRefusedOnlyOnProductionRoutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		demo     bool
		executor string
		refused  bool
	}{
		{"fixture on a production route", false, FixtureExecutor, true},
		{"fixture in demo mode", true, FixtureExecutor, false},
		{"native on a production route", false, "native", false},
		{"native in demo mode", true, "native", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RefuseFixture(tt.demo, tt.executor)
			if tt.refused != errors.Is(err, ErrFixtureRejected) || !tt.refused && err != nil {
				t.Fatalf("RefuseFixture(%v, %q) = %v, want refused=%v", tt.demo, tt.executor, err, tt.refused)
			}
		})
	}
}
