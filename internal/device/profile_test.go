package device_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

func TestEffectProfilesFenceReplayAndLiveLinks(t *testing.T) {
	t.Parallel()
	transport := idleLink{}
	cases := []struct {
		name   string
		config device.EffectProfileConfig
		valid  bool
	}{
		{"default simulated", device.EffectProfileConfig{}, true},
		{"simulated cannot carry link", device.EffectProfileConfig{GatewayLink: transport}, false},
		{"emulator needs link", device.EffectProfileConfig{Profile: device.EffectProfileEmulator}, false},
		{"emulator link", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayLink: transport}, true},
		{"emulator replay", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayLink: transport, ReplaySource: true}, false},
		{"physical replay", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: transport, ReplaySource: true, LiveActuation: true, OwnerAuthorized: true}, false},
		{"physical needs explicit live flag", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: transport, OwnerAuthorized: true}, false},
		{"physical needs owner", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: transport, LiveActuation: true}, false},
		{"emulator with configured options", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayOptions: true}, true},
		{"emulator options in replay", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayOptions: true, ReplaySource: true}, false},
		{"physical options need explicit live flag", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayOptions: true, OwnerAuthorized: true}, false},
		{"physical options need owner", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayOptions: true, LiveActuation: true}, false},
		{"physical options authorized", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayOptions: true, LiveActuation: true, OwnerAuthorized: true}, true},
		{"simulated options", device.EffectProfileConfig{GatewayOptions: true}, false},
		{"physical authorized", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: transport, LiveActuation: true, OwnerAuthorized: true}, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := device.ValidateEffectProfile(tc.config)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

// idleLink is a gateway link the profile check only needs to be present.
type idleLink struct{ device.Transport }
