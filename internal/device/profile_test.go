package device_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

type idleLink struct{ device.Transport }

func TestEffectProfileValidationReadsTheConfigurationItIsGiven(t *testing.T) {
	t.Parallel()
	link := idleLink{}
	cases := []struct {
		name   string
		config device.EffectProfileConfig
		want   string
	}{
		{"default is simulated", device.EffectProfileConfig{}, ""},
		{"a gateway link counts as a link", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayLink: link}, ""},
		{"configured gateway options count as a link", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayOptions: true}, ""},
		{"simulated cannot carry a link", device.EffectProfileConfig{GatewayLink: link}, "cannot configure a gateway link"},
		{"simulated cannot carry gateway options", device.EffectProfileConfig{GatewayOptions: true}, "cannot configure a gateway link"},
		{"emulator needs a link", device.EffectProfileConfig{Profile: device.EffectProfileEmulator}, "requires a gateway link"},
		{"replay source is passed through", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayLink: link, ReplaySource: true}, "replay or shadow"},
		{"shadow is passed through", device.EffectProfileConfig{Profile: device.EffectProfileEmulator, GatewayLink: link, Shadow: true}, "replay or shadow"},
		{"live actuation is passed through", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: link, OwnerAuthorized: true}, "explicit live actuation"},
		{"owner authorization is passed through", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: link, LiveActuation: true}, "owner authorization"},
		{"physical with consent", device.EffectProfileConfig{Profile: device.EffectProfilePhysical, GatewayLink: link, LiveActuation: true, OwnerAuthorized: true}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := device.ValidateEffectProfile(tc.config)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("ValidateEffectProfile = %v, want %q", err, tc.want)
			}
		})
	}
}
