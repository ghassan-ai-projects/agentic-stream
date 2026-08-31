package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type effectProfileOptions struct {
	Profile                actions.EffectProfile
	DeviceSocket           string
	DeviceCatalog          string
	AllowedFirmwareDigests []string
	LiveActuation          bool
	OwnerAuthorized        bool
}

// validate checks command-line profile inputs before opening a database or
// connecting to a device gateway. replaySource identifies the file-backed
// --trace path only; the normalized --live-socket source is intentionally not
// replay and may be used with the emulator or physical profile.
func (o effectProfileOptions) validate(replaySource bool) error {
	profile := o.profile()
	switch profile {
	case actions.EffectProfileSimulated:
		if o.hasGatewayConfiguration() {
			return fmt.Errorf("simulated effect profile cannot configure device gateway options")
		}
		return checkEffectProfile("validate simulated effect profile", actions.EffectProfileConfig{Profile: profile, ReplaySource: replaySource})
	case actions.EffectProfileEmulator, actions.EffectProfilePhysical:
		if replaySource {
			return checkEffectProfile("validate replay effect profile", actions.EffectProfileConfig{Profile: profile, ReplaySource: true})
		}
		if o.DeviceSocket == "" {
			return fmt.Errorf("%s effect profile requires --device-socket", profile)
		}
		if o.DeviceCatalog == "" {
			return fmt.Errorf("%s effect profile requires --device-catalog", profile)
		}
		if len(o.AllowedFirmwareDigests) == 0 {
			return fmt.Errorf("%s effect profile requires at least one --device-firmware-digest", profile)
		}
		if profile == actions.EffectProfilePhysical {
			if !o.LiveActuation {
				return fmt.Errorf("physical effect profile requires explicit live actuation")
			}
			if !o.OwnerAuthorized {
				return fmt.Errorf("physical effect profile requires owner authorization")
			}
		}
		return nil
	default:
		return checkEffectProfile("validate effect profile", actions.EffectProfileConfig{Profile: profile, ReplaySource: replaySource})
	}
}

func checkEffectProfile(prefix string, config actions.EffectProfileConfig) error {
	if err := actions.ValidateEffectProfile(config); err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return nil
}

// open creates the action-plane effector after the runtime owner has started.
// Device profiles use a typed UDS gateway link; the serial effector remains
// explicitly routed by runtime.NewPipeline.
func (o effectProfileOptions) open(
	ctx context.Context,
	db *storage.DB,
	owner *storage.RuntimeOwner,
	epochControl *storage.EpochControl,
	epoch string,
	telemetryRuntime *telemetry.Runtime,
	replaySource bool,
) (actions.Effector, *actions.SerialEffector, func() error, error) {
	if err := o.validate(replaySource); err != nil {
		return nil, nil, nil, err
	}
	if o.profile() == actions.EffectProfileSimulated {
		return actions.NewSimulatedEffector(), nil, nil, nil
	}

	catalogData, err := os.ReadFile(o.DeviceCatalog)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read device capability catalog: %w", err)
	}
	catalog, err := actions.LoadCapabilityCatalog(catalogData)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load device capability catalog: %w", err)
	}
	transport, err := actions.DialUDSTransport(ctx, o.DeviceSocket)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect device gateway: %w", err)
	}
	profileConfig := actions.EffectProfileConfig{
		Profile:         o.profile(),
		GatewayLink:     transport,
		LiveActuation:   o.LiveActuation,
		OwnerAuthorized: o.OwnerAuthorized,
	}
	if err := actions.ValidateEffectProfile(profileConfig); err != nil {
		_ = transport.Close()
		return nil, nil, nil, fmt.Errorf("validate effect profile: %w", err)
	}
	authority := &storage.TargetAuthority{
		DB: db, Owner: owner, EpochControl: epochControl, InstanceID: epoch, Lease: owner.Lease,
	}
	reconciliation := &storage.ReconciliationStore{DB: db, Authority: authority}
	serial, closeFn, err := actions.NewGatewayEffector(ctx, actions.GatewayEffectorConfig{
		Transport:              transport,
		Catalog:                catalog,
		AllowedFirmwareDigests: o.AllowedFirmwareDigests,
		AuthorityEpoch:         epoch,
		OwnerInstance:          epoch,
		Authority:              authority,
		Reconciliation:         reconciliation,
		Telemetry:              telemetryRuntime,
	})
	if err != nil {
		_ = transport.Close()
		return nil, nil, nil, fmt.Errorf("open gateway effector: %w", err)
	}
	return fallbackEffector(o.profile()), serial, closeFn, nil
}

func fallbackEffector(profile actions.EffectProfile) actions.Effector {
	if profile == actions.EffectProfilePhysical {
		return actions.NewFailClosedEffector(profile)
	}
	return actions.NewSimulatedEffector()
}

func (o effectProfileOptions) profile() actions.EffectProfile {
	if o.Profile == "" {
		return actions.EffectProfileSimulated
	}
	return o.Profile
}

func (o effectProfileOptions) hasGatewayConfiguration() bool {
	return o.DeviceSocket != "" || o.DeviceCatalog != "" || len(o.AllowedFirmwareDigests) > 0 || o.LiveActuation || o.OwnerAuthorized
}

func addEffectProfileFlags(
	cmd *cobra.Command,
	profile *string,
	deviceSocket *string,
	deviceCatalog *string,
	firmwareDigests *[]string,
	liveActuation *bool,
	ownerAuthorized *bool,
) {
	cmd.Flags().StringVar(profile, "effect-profile", string(actions.EffectProfileSimulated), "Effect profile: simulated, emulator, or physical")
	cmd.Flags().StringVar(deviceSocket, "device-socket", "", "Typed device gateway Unix socket for emulator or physical profiles")
	cmd.Flags().StringVar(deviceCatalog, "device-catalog", "", "Closed device capability catalog JSON path")
	cmd.Flags().StringSliceVar(firmwareDigests, "device-firmware-digest", nil, "Allow-listed device firmware digest (repeatable)")
	cmd.Flags().BoolVar(liveActuation, "live-actuation", false, "Explicitly permit physical actuation")
	cmd.Flags().BoolVar(ownerAuthorized, "owner-authorized", false, "Require explicit hardware-owner authorization for physical actuation")
}
