package actions

import "fmt"

// EffectProfile selects the action boundary used by a runtime process.
type EffectProfile string

const (
	// EffectProfileSimulated is the safe default and has no gateway link.
	EffectProfileSimulated EffectProfile = "simulated"
	// EffectProfileEmulator uses a typed gateway link backed by an emulator.
	EffectProfileEmulator EffectProfile = "emulator"
	// EffectProfilePhysical permits a real gateway link only with explicit
	// operator authorization.
	EffectProfilePhysical EffectProfile = "physical"
)

// EffectProfileConfig describes startup-time effect-boundary choices. It
// contains a typed link, never a raw serial path or credential.
type EffectProfileConfig struct {
	Profile         EffectProfile
	ReplaySource    bool
	Shadow          bool
	GatewayLink     DeviceTransport
	LiveActuation   bool
	OwnerAuthorized bool
}

// ValidateEffectProfile rejects configurations that could connect a replay or
// shadow process to an effect boundary. Physical actuation also requires an
// explicit live-actuation flag and owner authorization.
func ValidateEffectProfile(config EffectProfileConfig) error {
	profile := config.Profile
	if profile == "" {
		profile = EffectProfileSimulated
	}
	switch profile {
	case EffectProfileSimulated:
		if config.GatewayLink != nil {
			return fmt.Errorf("simulated effect profile cannot configure a gateway link")
		}
		return nil
	case EffectProfileEmulator:
		if config.ReplaySource || config.Shadow {
			return fmt.Errorf("emulator effect profile cannot be combined with replay or shadow")
		}
		if config.GatewayLink == nil {
			return fmt.Errorf("emulator effect profile requires a gateway link")
		}
		return nil
	case EffectProfilePhysical:
		if config.ReplaySource || config.Shadow {
			return fmt.Errorf("physical effect profile cannot be combined with replay or shadow")
		}
		if config.GatewayLink == nil {
			return fmt.Errorf("physical effect profile requires a gateway link")
		}
		if !config.LiveActuation {
			return fmt.Errorf("physical effect profile requires explicit live actuation")
		}
		if !config.OwnerAuthorized {
			return fmt.Errorf("physical effect profile requires owner authorization")
		}
		return nil
	default:
		return fmt.Errorf("unsupported effect profile %q", profile)
	}
}
