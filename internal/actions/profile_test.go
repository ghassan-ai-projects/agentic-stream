package actions_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
)

func TestEffectProfilesFenceReplayAndLiveLinks(t *testing.T) {
	t.Parallel()
	transport := &fakeDeviceTransport{}
	cases := []struct {
		name   string
		config actions.EffectProfileConfig
		valid  bool
	}{
		{"default simulated", actions.EffectProfileConfig{}, true},
		{"simulated cannot carry link", actions.EffectProfileConfig{GatewayLink: transport}, false},
		{"emulator needs link", actions.EffectProfileConfig{Profile: actions.EffectProfileEmulator}, false},
		{"emulator link", actions.EffectProfileConfig{Profile: actions.EffectProfileEmulator, GatewayLink: transport}, true},
		{"emulator replay", actions.EffectProfileConfig{Profile: actions.EffectProfileEmulator, GatewayLink: transport, ReplaySource: true}, false},
		{"physical replay", actions.EffectProfileConfig{Profile: actions.EffectProfilePhysical, GatewayLink: transport, ReplaySource: true, LiveActuation: true, OwnerAuthorized: true}, false},
		{"physical needs explicit live flag", actions.EffectProfileConfig{Profile: actions.EffectProfilePhysical, GatewayLink: transport, OwnerAuthorized: true}, false},
		{"physical needs owner", actions.EffectProfileConfig{Profile: actions.EffectProfilePhysical, GatewayLink: transport, LiveActuation: true}, false},
		{"physical authorized", actions.EffectProfileConfig{Profile: actions.EffectProfilePhysical, GatewayLink: transport, LiveActuation: true, OwnerAuthorized: true}, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := actions.ValidateEffectProfile(tc.config)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
