package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

func TestEffectProfileOptionsValidate(t *testing.T) {
	t.Parallel()
	firmware := []string{"sha256:" + strings.Repeat("a", 64)}
	cases := []struct {
		name       string
		options    effectProfileOptions
		replay     bool
		wantValid  bool
		wantReason string
	}{
		{name: "default simulated", wantValid: true},
		{
			name:       "simulated rejects gateway settings",
			options:    effectProfileOptions{DeviceSocket: "/tmp/device.sock"},
			wantReason: "simulated effect profile cannot configure device gateway options",
		},
		{
			name:       "emulator rejects replay",
			options:    effectProfileOptions{Profile: device.EffectProfileEmulator},
			replay:     true,
			wantReason: "emulator effect profile cannot be combined with replay or shadow",
		},
		{
			name:       "emulator requires socket",
			options:    effectProfileOptions{Profile: device.EffectProfileEmulator},
			wantReason: "emulator effect profile requires --device-socket",
		},
		{
			name:       "emulator requires catalog",
			options:    effectProfileOptions{Profile: device.EffectProfileEmulator, DeviceSocket: "/tmp/device.sock"},
			wantReason: "emulator effect profile requires --device-catalog",
		},
		{
			name: "emulator requires firmware allow-list",
			options: effectProfileOptions{
				Profile: device.EffectProfileEmulator, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
			},
			wantReason: "emulator effect profile requires at least one --device-firmware-digest",
		},
		{
			name: "emulator accepts live source",
			options: effectProfileOptions{
				Profile: device.EffectProfileEmulator, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
				AllowedFirmwareDigests: firmware,
			},
			wantValid: true,
		},
		{
			name: "physical requires live actuation",
			options: effectProfileOptions{
				Profile: device.EffectProfilePhysical, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
				AllowedFirmwareDigests: firmware, OwnerAuthorized: true,
			},
			wantReason: "physical effect profile requires explicit live actuation",
		},
		{
			name: "physical requires owner authorization",
			options: effectProfileOptions{
				Profile: device.EffectProfilePhysical, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
				AllowedFirmwareDigests: firmware, LiveActuation: true,
			},
			wantReason: "physical effect profile requires owner authorization",
		},
		{
			name: "physical configuration is valid",
			options: effectProfileOptions{
				Profile: device.EffectProfilePhysical, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
				AllowedFirmwareDigests: firmware, LiveActuation: true, OwnerAuthorized: true,
			},
			wantValid: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.options.validate(tc.replay)
			if tc.wantValid {
				if err != nil {
					t.Fatalf("validate returned error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("validate error = %v, want %q", err, tc.wantReason)
			}
		})
	}
}

func TestEffectProfileConsentComesFromTheDeviceRule(t *testing.T) {
	t.Parallel()
	base := effectProfileOptions{
		Profile: device.EffectProfilePhysical, DeviceSocket: "/tmp/device.sock", DeviceCatalog: "catalog.json",
		AllowedFirmwareDigests: []string{"sha256:" + strings.Repeat("a", 64)},
	}
	for _, tc := range []struct {
		name    string
		options effectProfileOptions
	}{
		{"no consent", base},
		{"no owner", withConsent(base, true, false)},
		{"no live actuation", withConsent(base, false, true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := device.ValidateEffectProfile(tc.options.profileConfig(false))
			err := tc.options.validate(false)
			if want == nil || errors.Unwrap(err) == nil || errors.Unwrap(err).Error() != want.Error() {
				t.Fatalf("validate = %v, device rule = %v", err, want)
			}
		})
	}
}

func withConsent(options effectProfileOptions, live, owner bool) effectProfileOptions {
	options.LiveActuation, options.OwnerAuthorized = live, owner
	return options
}
