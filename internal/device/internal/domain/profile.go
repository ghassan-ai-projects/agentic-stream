package domain

import "fmt"

// EffectProfile selects the effect boundary a runtime process may use.
type EffectProfile string

// Effect profiles.
const (
	// EffectProfileSimulated is the safe default and has no gateway link.
	EffectProfileSimulated EffectProfile = "simulated"
	// EffectProfileEmulator uses a gateway link backed by an emulator.
	EffectProfileEmulator EffectProfile = "emulator"
	// EffectProfilePhysical permits a real gateway link only with explicit
	// live actuation and owner authorization.
	EffectProfilePhysical EffectProfile = "physical"
)

// ProfileRequest is a process's startup choice of effect boundary.
type ProfileRequest struct {
	Profile         EffectProfile
	ReplaySource    bool
	Shadow          bool
	HasGatewayLink  bool
	LiveActuation   bool
	OwnerAuthorized bool
}

// CheckEffectProfile refuses a configuration that could connect a replay or
// shadow process to an effect boundary. An empty profile is simulated. A
// physical profile also requires explicit live actuation and owner
// authorization.
func CheckEffectProfile(request ProfileRequest) error {
	profile := request.Profile
	if profile == "" {
		profile = EffectProfileSimulated
	}
	switch profile {
	case EffectProfileSimulated:
		return requireNoGateway(request)
	case EffectProfileEmulator:
		return requireLiveGateway(profile, request)
	case EffectProfilePhysical:
		return requirePhysicalActuation(profile, request)
	default:
		return fmt.Errorf("unsupported effect profile %q", profile)
	}
}

func requirePhysicalActuation(profile EffectProfile, request ProfileRequest) error {
	if err := requireLiveGateway(profile, request); err != nil {
		return err
	}
	return requireActuationConsent(request)
}

func requireNoGateway(request ProfileRequest) error {
	if request.HasGatewayLink {
		return fmt.Errorf("simulated effect profile cannot configure a gateway link")
	}
	return nil
}

// requireLiveGateway keeps device profiles away from replay and shadow
// sources and requires a gateway link.
func requireLiveGateway(profile EffectProfile, request ProfileRequest) error {
	if request.ReplaySource || request.Shadow {
		return fmt.Errorf("%s effect profile cannot be combined with replay or shadow", profile)
	}
	if !request.HasGatewayLink {
		return fmt.Errorf("%s effect profile requires a gateway link", profile)
	}
	return nil
}

func requireActuationConsent(request ProfileRequest) error {
	if !request.LiveActuation {
		return fmt.Errorf("physical effect profile requires explicit live actuation")
	}
	if !request.OwnerAuthorized {
		return fmt.Errorf("physical effect profile requires owner authorization")
	}
	return nil
}
