package domain

import (
	"strings"
	"testing"
)

func TestCheckEffectProfile(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		request ProfileRequest
		want    string
	}{
		{"default is simulated", ProfileRequest{}, ""},
		{"simulated with a gateway", ProfileRequest{Profile: EffectProfileSimulated, HasGatewayLink: true}, "cannot configure a gateway link"},
		{"emulator", ProfileRequest{Profile: EffectProfileEmulator, HasGatewayLink: true}, ""},
		{"emulator without a gateway", ProfileRequest{Profile: EffectProfileEmulator}, "requires a gateway link"},
		{"emulator in replay", ProfileRequest{Profile: EffectProfileEmulator, HasGatewayLink: true, ReplaySource: true}, "replay or shadow"},
		{"physical without consent", ProfileRequest{Profile: EffectProfilePhysical, HasGatewayLink: true}, "explicit live actuation"},
		{"physical without owner", ProfileRequest{Profile: EffectProfilePhysical, HasGatewayLink: true, LiveActuation: true}, "owner authorization"},
		{"physical with consent", ProfileRequest{Profile: EffectProfilePhysical, HasGatewayLink: true, LiveActuation: true, OwnerAuthorized: true}, ""},
		{"physical in shadow", ProfileRequest{Profile: EffectProfilePhysical, HasGatewayLink: true, Shadow: true}, "replay or shadow"},
		{"unknown", ProfileRequest{Profile: "remote"}, "unsupported effect profile"},
	} {
		err := CheckEffectProfile(tt.request)
		if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: CheckEffectProfile = %v, want %q", tt.name, err, tt.want)
		}
	}
}
