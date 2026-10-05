package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// capabilityPhase runs a worker-aware mode's report-only work after the
// deterministic stream replay, with the session clock's current time.
type capabilityPhase func(ctx context.Context, session *replaySession, mode domain.Mode, caps domain.Capabilities, evaluationTime time.Time, result *domain.Result) error

// replaySession owns the isolated database, compiled policy and virtual
// clock shared by ingestion, stream processing and result collection.
type replaySession struct {
	database transport.Database
	store    store.Store
	compiled *spec.CompiledSpec
	log      *eventlog.EventLog
	clk      *clock.Virtual
	tenantID string
}

func prepareReplaySession(ctx context.Context, database transport.Database, specPath, tracePath, tenantID string) (*replaySession, error) {
	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return nil, fmt.Errorf("compile spec: %w", err)
	}

	session := &replaySession{database: database, store: store.New(database.DB), compiled: compiled, tenantID: tenantID}
	// Register the compiled schemas before deriving the virtual clock epoch.
	// Epoch derivation must inspect the same ingress validity boundary as
	// JSONLReplay; otherwise a quarantined future-dated line could move the
	// replay clock and change the result of valid evidence.
	if err := session.store.SaveSpecDeployment(ctx, tenantID, compiled); err != nil {
		return nil, fmt.Errorf("prepare replay deployment: %w", err)
	}
	log, clk, err := session.prepareReplayClock(ctx, tracePath)
	if err != nil {
		return nil, err
	}
	session.log, session.clk = log, clk
	return session, nil
}

func (s *replaySession) prepareReplayClock(ctx context.Context, tracePath string) (*eventlog.EventLog, *clock.Virtual, error) {
	requireSchemas := allInputSchemasDeclared(s.compiled)
	validationLog := eventlog.NewEventLog(s.database.DB)
	if requireSchemas {
		validationLog.RequireSchemaValidation()
	}
	epoch, err := s.traceEpoch(ctx, tracePath, validationLog)
	if err != nil {
		return nil, nil, fmt.Errorf("derive replay epoch: %w", err)
	}
	clk := clock.NewVirtual(epoch)
	log := eventlog.NewEventLogWithClock(s.database.DB, clk)
	// Register the compiled input schemas before replay ingestion. The stream
	// engine also enables this guard during construction, but doing it here is
	// essential: JSONLReplay is the boundary that quarantines malformed and
	// schema-invalid evidence before it can enter event_log.
	if requireSchemas {
		log.RequireSchemaValidation()
	}
	return log, clk, nil
}

func allInputSchemasDeclared(compiled *spec.CompiledSpec) bool {
	if len(compiled.Inputs) == 0 {
		return false
	}
	for _, input := range compiled.Inputs {
		if input.SchemaRef == "" {
			return false
		}
	}
	return true
}

func (s *replaySession) processTrace(ctx context.Context, tracePath string, cognitionEnabled bool) (int, error) {
	if err := s.ingestTrace(ctx, tracePath); err != nil {
		return 0, err
	}
	eng, err := s.newReplayEngine(ctx, cognitionEnabled)
	if err != nil {
		return 0, err
	}
	return s.runTraceEngine(ctx, eng, cognitionEnabled)
}

func (s *replaySession) ingestTrace(ctx context.Context, tracePath string) error {
	sources := transport.Sources{DB: s.database.DB, Log: s.log, TenantID: s.tenantID}
	if _, err := sources.RunJSONLTrace(ctx, tracePath, s.clk); err != nil {
		return fmt.Errorf("replay trace: %w", err)
	}
	return nil
}

func (s *replaySession) newReplayEngine(ctx context.Context, cognitionEnabled bool) (*engine.Service, error) {
	eng, err := engine.New(ctx, engine.Config{DB: s.database.DB, Log: s.log, Clock: s.clk, Spec: s.compiled, TenantID: s.tenantID,
		RuntimeOwner: engine.ReplayOwnership, Cognition: cognitionEnabled})
	if err != nil {
		return nil, fmt.Errorf("new engine: %w", err)
	}
	return eng, nil
}

func (s *replaySession) runTraceEngine(ctx context.Context, eng *engine.Service, cognitionEnabled bool) (int, error) {
	processed, err := s.runAllPartitions(ctx, eng)
	if err != nil {
		return 0, fmt.Errorf("run partitions: %w", err)
	}
	if err := s.materializeEpisodes(ctx, cognitionEnabled); err != nil {
		return 0, err
	}
	return processed, nil
}

func (s *replaySession) runAllPartitions(ctx context.Context, eng *engine.Service) (int, error) {
	count, err := eng.RunGlobal(ctx, s.advanceToRecordTime)
	if err != nil {
		return 0, fmt.Errorf("run global replay: %w", err)
	}
	return count, nil
}

func (s *replaySession) materializeEpisodes(ctx context.Context, cognitionEnabled bool) error {
	if !cognitionEnabled {
		return nil
	}
	if err := s.store.MaterializeEpisodes(ctx, s.compiled, s.tenantID, s.clk.Now()); err != nil {
		return fmt.Errorf("materialize replay episodes: %w", err)
	}
	return nil
}

func (s *replaySession) advanceToRecordTime(record eventlog.Record) error {
	processingTime := domain.RecordProcessingTime(record.IngestedAt, record.EventTime)
	if processingTime.After(s.clk.Now()) {
		s.clk.Advance(processingTime.Sub(s.clk.Now()))
	}
	return nil
}

func (s *replaySession) collectResult(ctx context.Context, processed int) (domain.Result, error) {
	versions, err := s.store.SituationVersionDigests(ctx, s.compiled.Digest)
	if err != nil {
		return domain.Result{}, fmt.Errorf("hash situations: %w", err)
	}
	return domain.Result{
		EventsProcessed: processed,
		VersionCount:    len(versions),
		VersionsHash:    domain.HashVersionDigests(versions),
		Mode:            domain.ModeDeterministic,
		EffectsAllowed:  false,
	}, nil
}
