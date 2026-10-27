package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

type plantWireFlags struct {
	environment, kindsDir, sitePath string
	hardwareBoards                  []string
	epoch                           int64
}

func registerPlantWireFlags(cmd *cobra.Command, socket *string, flags *plantWireFlags) {
	cmd.Flags().StringVar(socket, "plant-wire-socket", "", "owner-only Unix socket a plant wire node dials: an absolute path in a 0700 directory")
	cmd.Flags().StringVar(&flags.environment, "plant-wire-environment", plantwire.EnvironmentSimulated, "plant wire nodes accepted: simulated, hardware or mixed")
	cmd.Flags().StringVar(&flags.kindsDir, "plant-wire-kinds", "", "directory of the commissioned plant wire kind files (*.kind.json)")
	cmd.Flags().StringVar(&flags.sitePath, "plant-wire-site", "", "plant wire site file, kept outside the repository; its digest is logged")
	cmd.Flags().StringSliceVar(&flags.hardwareBoards, "hardware-boards", nil, "hardware boards the plant wire host may command (physical profile with consent; repeatable)")
	cmd.Flags().Int64Var(&flags.epoch, "plant-wire-epoch", 0, "plant wire authority epoch (default: Unix seconds at start)")
}

func (f plantWireFlags) validate(options effectProfileOptions) error {
	if options.PlantWireSocket == "" {
		return f.rejectWithoutSocket()
	}
	if f.kindsDir == "" {
		return fmt.Errorf("--plant-wire-socket requires --plant-wire-kinds")
	}
	if err := plantwire.CheckSettings(f.settings(options)); err != nil {
		return fmt.Errorf("plant wire settings: %w", err)
	}
	return nil
}

func (f plantWireFlags) rejectWithoutSocket() error {
	configured := f.environment != "" && f.environment != plantwire.EnvironmentSimulated
	if configured || f.kindsDir != "" || f.sitePath != "" || len(f.hardwareBoards) > 0 || f.epoch != 0 {
		return fmt.Errorf("--plant-wire-environment, --plant-wire-kinds, --plant-wire-site, --plant-wire-epoch and --hardware-boards require --plant-wire-socket")
	}
	return nil
}

func (f plantWireFlags) settings(options effectProfileOptions) plantwire.Settings {
	consent := device.GrantsActuationConsent(options.profileConfig(false))
	return plantwire.Settings{Environment: f.environment, HardwareBoards: f.hardwareBoards, PhysicalConsent: consent, Epoch: f.epoch}
}

func (core *runtimeCore) openServeEffects(ctx context.Context, flags serveFlags, metrics *telemetry.Runtime, cleanup *cleanups) (effects, error) {
	opened, err := core.openEffects(ctx, flags.profileOptions(), metrics, flags.tracePath != "", cleanup)
	if err != nil {
		return effects{}, err
	}
	opened.plant, err = core.openPlantWireHost(flags, cleanup)
	if opened.plant != nil && flags.profileOptions().DeviceCatalog != "" {
		opened.device = opened.plant
	}
	return opened, err
}

type plantWireCommanding struct {
	catalog   *device.CapabilityCatalog
	authority *deviceauthority.Service
	epoch     string
}

func (core *runtimeCore) openPlantWireHost(flags serveFlags, cleanup *cleanups) (*plantwire.Host, error) {
	options := flags.profileOptions()
	if options.PlantWireSocket == "" {
		return nil, nil
	}
	commanding, err := core.plantWireCommanding(options)
	if err != nil {
		return nil, err
	}
	host, err := newPlantWireHost(flags.plantWire, options, commanding)
	if err != nil {
		return nil, err
	}
	cleanup.add(func() { _ = host.Close() })
	return host, nil
}

func (core *runtimeCore) plantWireCommanding(options effectProfileOptions) (plantWireCommanding, error) {
	catalog, err := plantWireCatalog(options)
	if err != nil || catalog == nil {
		return plantWireCommanding{}, err
	}
	authority, err := newDeviceAuthority(core.db, core.owner, core.epochControl)
	if err != nil {
		return plantWireCommanding{}, err
	}
	return plantWireCommanding{catalog: catalog, authority: authority, epoch: core.epoch}, nil
}

func plantWireCatalog(options effectProfileOptions) (*device.CapabilityCatalog, error) {
	if options.DeviceCatalog == "" {
		return nil, nil
	}
	catalog, err := options.loadDeviceCatalog()
	if err != nil {
		return nil, err
	}
	if err := catalog.CheckExpiriesWithin(plantWireCallTimeout(options)); err != nil {
		return nil, fmt.Errorf("check the plant wire routes against --dispatch-lease: %w", err)
	}
	return catalog, nil
}

func plantWireCallTimeout(options effectProfileOptions) time.Duration {
	call := actions.DispatchCallTimeout(options.DispatchLease)
	return call - plantwire.AnswerGrace - plantwire.VerificationWindow
}

func newPlantWireHost(flags plantWireFlags, options effectProfileOptions, commanding plantWireCommanding) (*plantwire.Host, error) {
	listener, err := worker.ListenEvidenceSocket(options.PlantWireSocket)
	if err != nil {
		return nil, fmt.Errorf("listen on the plant wire socket: %w", err)
	}
	host, err := plantwire.New(plantwire.Config{Listener: listener, Settings: flags.settings(options),
		KindsDir: flags.kindsDir, SitePath: flags.sitePath, Catalog: commanding.catalog, Authority: commanding.authority,
		OwnerEpoch: commanding.epoch})
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("open the plant wire host: %w", err)
	}
	return host, nil
}

func runPlantWireHost(host *plantwire.Host, pipeline *runtime.Pipeline, run func(string, func(context.Context) error)) {
	if host != nil {
		run("plant wire host", func(ctx context.Context) error { return pipeline.RunFeed(ctx, host.Run) })
	}
}

func (e effects) kindDigests() func() map[string]string {
	if e.plant == nil {
		return nil
	}
	return e.plant.BornKindDigests
}
