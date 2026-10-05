package device

import "github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

// EffectProfile selects the effect boundary a runtime process may use.
type EffectProfile = domain.EffectProfile

// Effect profiles.
const (
	EffectProfileSimulated = domain.EffectProfileSimulated
	EffectProfileEmulator  = domain.EffectProfileEmulator
	EffectProfilePhysical  = domain.EffectProfilePhysical
)

// EffectProfileConfig describes startup-time effect-boundary choices. It
// contains a typed link, never a raw serial path or credential.
type EffectProfileConfig struct {
	Profile         EffectProfile
	ReplaySource    bool
	Shadow          bool
	GatewayLink     Transport
	LiveActuation   bool
	OwnerAuthorized bool
}

// ValidateEffectProfile rejects configurations that could connect a replay or
// shadow process to an effect boundary. Physical actuation also requires an
// explicit live-actuation flag and owner authorization.
func ValidateEffectProfile(config EffectProfileConfig) error {
	return domain.CheckEffectProfile(domain.ProfileRequest{
		Profile: config.Profile, ReplaySource: config.ReplaySource, Shadow: config.Shadow,
		HasGatewayLink: config.GatewayLink != nil, LiveActuation: config.LiveActuation, OwnerAuthorized: config.OwnerAuthorized,
	})
}
