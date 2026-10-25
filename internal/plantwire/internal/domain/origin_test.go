package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestAHardwareDeviceIsCommandedOnlyWithConsentAndItsBoardNamed(t *testing.T) {
	t.Parallel()
	consenting := domain.Settings{Environment: domain.EnvironmentMixed, HardwareBoards: []string{"mega-01"}, PhysicalConsent: true}
	unconsenting := consenting
	unconsenting.PhysicalConsent = false
	cases := []struct {
		name     string
		settings domain.Settings
		origin   domain.Origin
		allowed  bool
	}{
		{"a simulated board", domain.Settings{Environment: domain.EnvironmentSimulated}, domain.Origin{Board: "sim-01", Environment: domain.EnvironmentSimulated}, true},
		{"a named hardware board with consent", consenting, domain.Origin{Board: "mega-01", Environment: domain.EnvironmentHardware}, true},
		{"a named hardware board without consent", unconsenting, domain.Origin{Board: "mega-01", Environment: domain.EnvironmentHardware}, false},
		{"a hardware board not named", consenting, domain.Origin{Board: "mega-02", Environment: domain.EnvironmentHardware}, false},
		{"a board the node did not announce", consenting, domain.Origin{Board: "mega-01"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.settings.Authorizes(tc.origin)
			if allowed := err == nil; allowed != tc.allowed {
				t.Fatalf("Authorizes(%+v) = %v, want allowed %v", tc.origin, err, tc.allowed)
			}
			if err != nil && (!errors.Is(err, domain.ErrOriginNotAuthorized) || !strings.Contains(err.Error(), tc.origin.Board)) {
				t.Fatalf("refusal = %v, want ErrOriginNotAuthorized naming the board", err)
			}
		})
	}
}
