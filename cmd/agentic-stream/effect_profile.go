package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type effectProfileOptions struct {
	Profile                device.EffectProfile
	DeviceSocket           string
	DeviceCatalog          string
	AllowedFirmwareDigests []string
	LiveActuation          bool
	OwnerAuthorized        bool
}

// workerRuntimeFlagTargets contains the command-local storage for the shared
// worker, evidence, and model flags. Keeping registration here prevents the
// run-live and serve commands from drifting apart.
type workerRuntimeFlagTargets struct {
	workerSocket     *string
	modelEndpoint    *string
	modelName        *string
	workerName       *string
	workerCA         *string
	workerCert       *string
	workerKey        *string
	workerServerName *string
	evidenceSocket   *string
	evidenceKey      *string
}

// validate checks command-line profile inputs before opening a database or
// connecting to a device gateway. replaySource identifies the file-backed
// --trace path only; the normalized --live-socket source is intentionally not
// replay and may be used with the emulator or physical profile.
func (o effectProfileOptions) validate(replaySource bool) error {
	profile := o.profile()
	switch profile {
	case device.EffectProfileSimulated:
		if o.hasGatewayConfiguration() {
			return fmt.Errorf("simulated effect profile cannot configure device gateway options")
		}
		return checkEffectProfile("validate simulated effect profile", device.EffectProfileConfig{Profile: profile, ReplaySource: replaySource})
	case device.EffectProfileEmulator, device.EffectProfilePhysical:
		if replaySource {
			return checkEffectProfile("validate replay effect profile", device.EffectProfileConfig{Profile: profile, ReplaySource: true})
		}
		return o.validateGatewayOptions(profile)
	default:
		return checkEffectProfile("validate effect profile", device.EffectProfileConfig{Profile: profile, ReplaySource: replaySource})
	}
}

func (o effectProfileOptions) validateGatewayOptions(profile device.EffectProfile) error {
	if o.DeviceSocket == "" {
		return fmt.Errorf("%s effect profile requires --device-socket", profile)
	}
	if o.DeviceCatalog == "" {
		return fmt.Errorf("%s effect profile requires --device-catalog", profile)
	}
	if len(o.AllowedFirmwareDigests) == 0 {
		return fmt.Errorf("%s effect profile requires at least one --device-firmware-digest", profile)
	}
	return o.validatePhysicalAuthorization(profile)
}

func (o effectProfileOptions) validatePhysicalAuthorization(profile device.EffectProfile) error {
	if profile == device.EffectProfilePhysical {
		if !o.LiveActuation {
			return fmt.Errorf("physical effect profile requires explicit live actuation")
		}
		if !o.OwnerAuthorized {
			return fmt.Errorf("physical effect profile requires owner authorization")
		}
	}
	return nil
}

func checkEffectProfile(prefix string, config device.EffectProfileConfig) error {
	if err := device.ValidateEffectProfile(config); err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return nil
}

// open creates the action-plane effector after the runtime owner has started.
// Device profiles use a typed UDS gateway link; the serial effector remains
// explicitly routed by runtime.NewPipeline.
func (o effectProfileOptions) open(ctx context.Context, db *storage.DB, owner *runtimecontrol.RuntimeOwner, epochControl *runtimecontrol.EpochControl, epoch string, telemetryRuntime *telemetry.Runtime, replaySource bool) (actionport.Effector, *device.SerialEffector, func() error, error) {
	if err := o.validate(replaySource); err != nil {
		return nil, nil, nil, err
	}
	if o.profile() == device.EffectProfileSimulated {
		return device.NewSimulatedEffector(), nil, nil, nil
	}

	transport, catalog, err := o.connectDeviceGateway(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return o.openGatewayEffector(ctx, db, owner, epochControl, epoch, telemetryRuntime, transport, catalog)
}

func (o effectProfileOptions) connectDeviceGateway(ctx context.Context) (*device.UDSTransport, *device.CapabilityCatalog, error) {
	catalog, err := o.loadDeviceCatalog()
	if err != nil {
		return nil, nil, err
	}
	transport, err := device.DialUDSTransport(ctx, o.DeviceSocket)
	if err != nil {
		return nil, nil, fmt.Errorf("connect device gateway: %w", err)
	}
	if err := o.validateGatewayLink(transport); err != nil {
		_ = transport.Close()
		return nil, nil, err
	}
	return transport, catalog, nil
}

func (o effectProfileOptions) loadDeviceCatalog() (*device.CapabilityCatalog, error) {
	catalogData, err := os.ReadFile(o.DeviceCatalog)
	if err != nil {
		return nil, fmt.Errorf("read device capability catalog: %w", err)
	}
	catalog, err := device.LoadCapabilityCatalog(catalogData)
	if err != nil {
		return nil, fmt.Errorf("load device capability catalog: %w", err)
	}
	return catalog, nil
}

func (o effectProfileOptions) validateGatewayLink(transport *device.UDSTransport) error {
	profileConfig := device.EffectProfileConfig{
		Profile:         o.profile(),
		GatewayLink:     transport,
		LiveActuation:   o.LiveActuation,
		OwnerAuthorized: o.OwnerAuthorized,
	}
	if err := device.ValidateEffectProfile(profileConfig); err != nil {
		return fmt.Errorf("validate effect profile: %w", err)
	}
	return nil
}

func (o effectProfileOptions) openGatewayEffector(ctx context.Context, db *storage.DB, owner *runtimecontrol.RuntimeOwner, epochControl *runtimecontrol.EpochControl, epoch string, telemetryRuntime *telemetry.Runtime, transport *device.UDSTransport, catalog *device.CapabilityCatalog) (actionport.Effector, *device.SerialEffector, func() error, error) {
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: epochControl, Outcomes: actions.CountUnresolvedOutcomes, ClaimLease: owner.Lease})
	if err != nil {
		_ = transport.Close()
		return nil, nil, nil, fmt.Errorf("configure device authority: %w", err)
	}
	serial, closeFn, err := device.NewGatewayEffector(ctx, o.gatewayEffectorConfig(authority, epoch, telemetryRuntime, transport, catalog))
	if err != nil {
		_ = transport.Close()
		return nil, nil, nil, fmt.Errorf("open gateway effector: %w", err)
	}
	return fallbackEffector(o.profile()), serial, closeFn, nil
}

func (o effectProfileOptions) gatewayEffectorConfig(authority *deviceauthority.Service, epoch string, telemetryRuntime *telemetry.Runtime, transport *device.UDSTransport, catalog *device.CapabilityCatalog) device.GatewayEffectorConfig {
	return device.GatewayEffectorConfig{
		Transport:              transport,
		Catalog:                catalog,
		AllowedFirmwareDigests: o.AllowedFirmwareDigests,
		OwnerEpoch:             epoch,
		OwnerInstance:          epoch,
		Authority:              authority,
		Telemetry:              telemetryRuntime,
	}
}

func fallbackEffector(profile device.EffectProfile) actionport.Effector {
	if profile == device.EffectProfilePhysical {
		return device.NewFailClosedEffector(profile)
	}
	return device.NewSimulatedEffector()
}

func (o effectProfileOptions) profile() device.EffectProfile {
	if o.Profile == "" {
		return device.EffectProfileSimulated
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
	cmd.Flags().StringVar(profile, "effect-profile", string(device.EffectProfileSimulated), "Effect profile: simulated, emulator, or physical")
	cmd.Flags().StringVar(deviceSocket, "device-socket", "", "Typed device gateway Unix socket for emulator or physical profiles")
	cmd.Flags().StringVar(deviceCatalog, "device-catalog", "", "Closed device capability catalog JSON path")
	cmd.Flags().StringSliceVar(firmwareDigests, "device-firmware-digest", nil, "Allow-listed device firmware digest (repeatable)")
	cmd.Flags().BoolVar(liveActuation, "live-actuation", false, "Explicitly permit physical actuation")
	cmd.Flags().BoolVar(ownerAuthorized, "owner-authorized", false, "Require explicit hardware-owner authorization for physical actuation")
}

func addWorkerRuntimeFlags(cmd *cobra.Command, targets workerRuntimeFlagTargets) {
	cmd.Flags().StringVar(targets.workerSocket, "worker-socket", "", "EpisodeWorker Unix socket (overrides the native Go executor)")
	cmd.Flags().StringVar(targets.modelEndpoint, "model-endpoint", "", "OpenAI-compatible model endpoint for the native Go executor")
	cmd.Flags().StringVar(targets.modelName, "model-name", "", "Model name for the OpenAI-compatible native provider")
	cmd.Flags().StringVar(targets.workerName, "worker-name", "native", "Expected EpisodeWorker name")
	cmd.Flags().StringVar(targets.workerCA, "worker-ca", "", "Worker CA PEM (enables mTLS)")
	cmd.Flags().StringVar(targets.workerCert, "worker-cert", "", "Runtime client certificate PEM")
	cmd.Flags().StringVar(targets.workerKey, "worker-key", "", "Runtime client private key PEM")
	cmd.Flags().StringVar(targets.workerServerName, "worker-server-name", "", "Expected worker certificate name")
	cmd.Flags().StringVar(targets.evidenceSocket, "evidence-socket", "", "Runtime EvidenceTools Unix socket for worker episodes")
	cmd.Flags().StringVar(targets.evidenceKey, "evidence-key", "", "Hex HMAC key shared with the runtime EvidenceTools verifier")
}
