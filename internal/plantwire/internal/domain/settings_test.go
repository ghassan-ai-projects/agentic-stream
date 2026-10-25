package domain_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestCheckSettingsRefusesAHostWithoutTheConsentItsBoardsNeed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		settings domain.Settings
		want     error
	}{
		{"a simulated host", domain.Settings{Environment: domain.EnvironmentSimulated}, nil},
		{"a mixed host with consent and one board", domain.Settings{Environment: domain.EnvironmentMixed, HardwareBoards: []string{"mega-01"}, PhysicalConsent: true, Epoch: 5}, nil},
		{"an unknown environment", domain.Settings{Environment: "lab"}, domain.ErrUnknownEnvironment},
		{"a negative epoch", domain.Settings{Environment: domain.EnvironmentSimulated, Epoch: -1}, domain.ErrNegativeEpoch},
		{"a board name with prose", domain.Settings{Environment: domain.EnvironmentHardware, HardwareBoards: []string{"mega 01"}, PhysicalConsent: true}, domain.ErrBoardName},
		{"a board named twice", domain.Settings{Environment: domain.EnvironmentHardware, HardwareBoards: []string{"mega-01", "mega-01"}, PhysicalConsent: true}, domain.ErrBoardTwice},
		{"hardware boards on a simulated host", domain.Settings{Environment: domain.EnvironmentSimulated, HardwareBoards: []string{"mega-01"}, PhysicalConsent: true}, domain.ErrBoardsOnSimulatedHost},
		{"a hardware host without consent", domain.Settings{Environment: domain.EnvironmentHardware}, domain.ErrHardwareWithoutConsent},
		{"a mixed host without consent", domain.Settings{Environment: domain.EnvironmentMixed}, domain.ErrHardwareWithoutConsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := domain.CheckSettings(tc.settings); !errors.Is(err, tc.want) || (tc.want == nil) != (err == nil) {
				t.Fatalf("CheckSettings = %v, want %v", err, tc.want)
			}
		})
	}
}
