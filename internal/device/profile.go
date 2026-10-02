package device

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
		return requireLiveGateway(profile, config)
	case EffectProfilePhysical:
		if err := requireLiveGateway(profile, config); err != nil {
			return err
		}
		return requireActuationConsent(config)
	default:
		return fmt.Errorf("unsupported effect profile %q", profile)
	}
}

// requireLiveGateway keeps device profiles away from replay and shadow
// sources and requires a gateway link.
func requireLiveGateway(profile EffectProfile, config EffectProfileConfig) error {
	if config.ReplaySource || config.Shadow {
		return fmt.Errorf("%s effect profile cannot be combined with replay or shadow", profile)
	}
	if config.GatewayLink == nil {
		return fmt.Errorf("%s effect profile requires a gateway link", profile)
	}
	return nil
}

// requireActuationConsent requires both explicit live actuation and owner
// authorization for physical effects.
func requireActuationConsent(config EffectProfileConfig) error {
	if !config.LiveActuation {
		return fmt.Errorf("physical effect profile requires explicit live actuation")
	}
	if !config.OwnerAuthorized {
		return fmt.Errorf("physical effect profile requires owner authorization")
	}
	return nil
}
