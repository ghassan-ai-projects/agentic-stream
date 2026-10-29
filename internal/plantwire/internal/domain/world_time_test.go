package domain_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestOnlyASimulatedNodeNotPacedToTheWallIsAPacedWorld(t *testing.T) {
	t.Parallel()
	cases := []struct {
		environment, clockMode string
		want                   bool
	}{
		{domain.EnvironmentSimulated, "", true},
		{domain.EnvironmentSimulated, "virtual", true},
		{domain.EnvironmentSimulated, domain.ClockModeWall, false},
		{domain.EnvironmentHardware, "", false},
		{domain.EnvironmentHardware, "virtual", false},
		{domain.EnvironmentMixed, "virtual", false},
	}
	for _, tc := range cases {
		t.Run(tc.environment+"/"+tc.clockMode, func(t *testing.T) {
			t.Parallel()
			birth := birthOf(t, func(b *domain.Birth) { b.Environment, b.NodeMetrics.ClockMode = tc.environment, tc.clockMode })
			if got := birth.PacedWorld(); got != tc.want {
				t.Fatalf("PacedWorld() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPacedWorldReportsItsBarrierWorldTimeAndAWallPacedOneDoesNot(t *testing.T) {
	t.Parallel()
	for _, clockMode := range []string{"virtual", domain.ClockModeWall} {
		t.Run(clockMode, func(t *testing.T) {
			t.Parallel()
			conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
			birth := birthOf(t, func(b *domain.Birth) { b.NodeMetrics.ClockMode = clockMode })
			conversation.Receive(domain.Arrival{Envelope: birth.Envelope, Sequenced: true, Birth: &birth})
			world := conversation.Receive(syncArrival(1)).World
			if clockMode == domain.ClockModeWall {
				if world != nil {
					t.Fatalf("World = %+v, want none for a wall-paced node", world)
				}
				return
			}
			if want := time.Unix(0, 1).UTC(); world == nil || !world.AtBarrier || !world.At.Equal(want) {
				t.Fatalf("World = %+v, want the barrier world time %v", world, want)
			}
		})
	}
}
