package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// liveFlags are the flags run-live and serve share. Registering them in one
// place keeps the two live commands from drifting apart.
type liveFlags struct {
	dbPath, specPath, tracePath, tenantID, traceFormat string
	effectProfile                                      string
	effect                                             effectProfileOptions
	worker                                             runtime.WorkerRuntimeConfig
}

// cleanups runs registered release functions in reverse order, like a
// sequence of defers that helper functions can extend.
type cleanups []func()

// runtimeCore is the owner-fenced runtime both live commands start from: the
// database, a fresh runtime epoch with its owner lease and evidence ledger,
// epoch control, and the started runtime service.
type runtimeCore struct {
	pipeline     *runtime.Pipeline
	db           *storage.DB
	epoch        string
	owner        *runtimecontrol.RuntimeOwner
	ledger       *evidence.Service
	epochControl *runtimecontrol.EpochControl
	service      *runtime.Service
}

// effects is the opened effect profile.
type effects struct {
	effector actionport.Effector
	serial   *device.GatewayEffector
}

// registerShared adds the effect-profile and worker-runtime flags.
func (f *liveFlags) registerShared(cmd *cobra.Command) {
	addEffectProfileFlags(cmd, &f.effectProfile, &f.effect.DeviceSocket, &f.effect.DeviceCatalog,
		&f.effect.AllowedFirmwareDigests, &f.effect.LiveActuation, &f.effect.OwnerAuthorized)
	addWorkerRuntimeFlags(cmd, workerRuntimeFlagTargets{
		workerSocket: &f.worker.WorkerSocket, modelEndpoint: &f.worker.ModelEndpoint, modelName: &f.worker.ModelName,
		workerName: &f.worker.WorkerName, workerCA: &f.worker.WorkerCA, workerCert: &f.worker.WorkerCert,
		workerKey: &f.worker.WorkerKey, workerServerName: &f.worker.WorkerServerName,
		evidenceSocket: &f.worker.EvidenceSocket, evidenceKey: &f.worker.EvidenceKey,
	})
}

// profileOptions returns the effect-profile options with the parsed profile.
func (f *liveFlags) profileOptions() effectProfileOptions {
	options := f.effect
	options.Profile = device.EffectProfile(f.effectProfile)
	return options
}

func (c *cleanups) add(fn func()) { *c = append(*c, fn) }

func (c *cleanups) run() {
	for i := len(*c) - 1; i >= 0; i-- {
		(*c)[i]()
	}
}

func openRuntimeCore(ctx context.Context, dbPath string, lease time.Duration, cleanup *cleanups) (*runtimeCore, error) {
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open runtime database: %w", err)
	}
	cleanup.add(func() { _ = db.Close() })
	core, err := newRuntimeCore(db, lease)
	if err != nil {
		return nil, err
	}
	if err := core.startService(ctx, cleanup); err != nil {
		return nil, err
	}
	return core, nil
}

func newRuntimeCore(db *storage.DB, lease time.Duration) (*runtimeCore, error) {
	epoch, err := evidence.NewRuntimeEpoch()
	if err != nil {
		return nil, fmt.Errorf("generate runtime epoch: %w", err)
	}
	core := &runtimeCore{
		db: db, epoch: epoch,
		owner:        &runtimecontrol.RuntimeOwner{DB: db, InstanceID: epoch, Lease: lease},
		epochControl: &runtimecontrol.EpochControl{DB: db},
	}
	core.ledger, err = evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{DB: db, OwnerCheck: core.owner.Assert, LeaseOwner: epoch, RuntimeEpoch: epoch, Lease: lease}})
	if err != nil {
		return nil, fmt.Errorf("configure evidence ledger: %w", err)
	}
	return core, nil
}

func (core *runtimeCore) startService(ctx context.Context, cleanup *cleanups) error {
	var err error
	core.service, err = runtime.NewService(core.owner, core.ledger, core.epoch)
	if err != nil {
		return fmt.Errorf("create runtime service: %w", err)
	}
	if _, err := core.service.Start(ctx); err != nil {
		return fmt.Errorf("start runtime: %w", err)
	}
	cleanup.add(func() { _ = core.service.Close(context.Background()) })
	return nil
}

func (core *runtimeCore) openEffects(ctx context.Context, options effectProfileOptions, metrics *telemetry.Runtime, replaySource bool, cleanup *cleanups) (effects, error) {
	effector, gatewayEffector, closeEffector, err := options.open(ctx, core.db, core.owner, core.epochControl, core.epoch, metrics, replaySource)
	if err != nil {
		return effects{}, fmt.Errorf("configure effect profile: %w", err)
	}
	if closeEffector != nil {
		cleanup.add(func() { _ = closeEffector() })
	}
	return effects{effector: effector, serial: gatewayEffector}, nil
}

func (core *runtimeCore) openWorkerRuntime(ctx context.Context, config runtime.WorkerRuntimeConfig, cleanup *cleanups) (*runtime.WorkerRuntime, error) {
	config.DB = core.db
	config.Ledger = core.ledger
	config.RuntimeEpoch = core.epoch
	workerRuntime, err := runtime.NewWorkerRuntime(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("configure worker runtime: %w", err)
	}
	cleanup.add(func() { _ = workerRuntime.Close() })
	return workerRuntime, nil
}

func (core *runtimeCore) startPipeline(ctx context.Context, compiled *spec.CompiledSpec, tenantID string, workerRuntime *runtime.WorkerRuntime, opened effects, metrics *telemetry.Runtime, demoMode bool, cleanup *cleanups) (*runtime.Pipeline, error) {
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: core.db, Spec: compiled, TenantID: tenantID, Owner: core.owner, OwnerEpoch: core.epoch,
		Executor: workerRuntime.Executor, Effector: opened.effector, GatewayEffector: opened.serial,
		IDGenerator: sources.Random(), Telemetry: metrics, EpochControl: core.epochControl, DemoMode: demoMode,
	})
	if err != nil {
		return nil, fmt.Errorf("create runtime pipeline: %w", err)
	}
	if err := pipeline.Start(ctx); err != nil {
		return nil, fmt.Errorf("start pipeline maintenance: %w", err)
	}
	cleanup.add(func() { _ = pipeline.Close() })
	return pipeline, nil
}

// runTrace runs one batch of a JSONL trace in the given format.
func runTrace(ctx context.Context, pipeline *runtime.Pipeline, format, path string) (runtime.PipelineReport, error) {
	runs := map[string]func(context.Context, string) (runtime.PipelineReport, error){
		"normalized": pipeline.RunJSONL,
		"simulator":  pipeline.RunSimulatorJSONL,
	}
	run, ok := runs[format]
	if !ok {
		return runtime.PipelineReport{}, fmt.Errorf("unsupported --trace-format %q", format)
	}
	report, err := run(ctx, path)
	if err != nil {
		return report, fmt.Errorf("run %s trace: %w", format, err)
	}
	return report, nil
}

// pollTrace reruns an append-only trace every interval until ctx ends. It
// returns nil on cancellation and the first other error.
func pollTrace(ctx context.Context, pipeline *runtime.Pipeline, format, path string, interval time.Duration) error {
	for {
		if _, err := runTrace(ctx, pipeline, format, path); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
