package replay

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// replaySession owns the isolated database, compiled policy, and virtual clock
// shared by ingestion, stream processing, and result collection.
type replaySession struct {
	db       *storage.DB
	compiled *spec.CompiledSpec
	log      *eventlog.EventLog
	clk      *clock.Virtual
	tenantID string
}

func prepareReplaySession(ctx context.Context, db *storage.DB, specPath, tracePath, tenantID string) (*replaySession, error) {
	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return nil, fmt.Errorf("compile spec: %w", err)
	}

	// Register the compiled schemas before deriving the virtual clock epoch.
	// Epoch derivation must inspect the same ingress validity boundary as
	// JSONLReplay; otherwise a quarantined future-dated line could move the
	// replay clock and change the result of valid evidence.
	if err := spec.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return nil, fmt.Errorf("prepare replay deployment: %w", err)
	}
	log, clk, err := prepareReplayClock(ctx, db, compiled, tracePath, tenantID)
	if err != nil {
		return nil, err
	}
	return &replaySession{db: db, compiled: compiled, log: log, clk: clk, tenantID: tenantID}, nil
}

func prepareReplayClock(ctx context.Context, db *storage.DB, compiled *spec.CompiledSpec, tracePath, tenantID string) (*eventlog.EventLog, *clock.Virtual, error) {
	requireSchemas := allInputSchemasDeclared(compiled)
	validationLog := eventlog.NewEventLog(db)
	if requireSchemas {
		validationLog.RequireSchemaValidation()
	}
	epoch, err := traceEpoch(ctx, tracePath, tenantID, validationLog)
	if err != nil {
		return nil, nil, fmt.Errorf("derive replay epoch: %w", err)
	}
	clk := clock.NewVirtual(epoch)
	log := eventlog.NewEventLogWithClock(db, clk)
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
	conn := ingress.NewJSONLReplayWithClock(s.db, s.log, s.tenantID, tracePath, "replay:"+tracePath, s.clk)
	if _, err := conn.Run(ctx); err != nil {
		return fmt.Errorf("replay trace: %w", err)
	}
	return nil
}

func (s *replaySession) newReplayEngine(ctx context.Context, cognitionEnabled bool) (*engine.Engine, error) {
	var eng *engine.Engine
	var err error
	if cognitionEnabled {
		eng, err = engine.NewEngine(ctx, s.db, s.log, s.clk, s.compiled, s.tenantID)
	} else {
		eng, err = engine.NewStreamEngine(ctx, s.db, s.log, s.clk, s.compiled, s.tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("new engine: %w", err)
	}
	return eng, nil
}

func (s *replaySession) runTraceEngine(ctx context.Context, eng *engine.Engine, cognitionEnabled bool) (int, error) {
	processed, err := runAllPartitions(ctx, eng, s.advanceToRecordTime)
	if err != nil {
		return 0, fmt.Errorf("run partitions: %w", err)
	}
	if err := s.materializeEpisodes(ctx, cognitionEnabled); err != nil {
		return 0, err
	}
	return processed, nil
}

func (s *replaySession) materializeEpisodes(ctx context.Context, cognitionEnabled bool) error {
	if cognitionEnabled {
		if err := materializeReplayEpisodes(ctx, s.db, s.compiled, s.tenantID, s.clk.Now()); err != nil {
			return fmt.Errorf("materialize replay episodes: %w", err)
		}
	}
	return nil
}

func (s *replaySession) advanceToRecordTime(record eventlog.Record) error {
	processingTime := record.IngestedAt.UTC()
	if processingTime.IsZero() {
		processingTime = record.EventTime.UTC()
	}
	if processingTime.After(s.clk.Now()) {
		s.clk.Advance(processingTime.Sub(s.clk.Now()))
	}
	return nil
}

func (s *replaySession) collectResult(ctx context.Context, processed int) (Result, error) {
	versionsHash, versionCount, err := hashSituationVersions(ctx, s.db, s.compiled.Digest)
	if err != nil {
		return Result{}, fmt.Errorf("hash situations: %w", err)
	}

	result := Result{
		EventsProcessed: processed,
		VersionCount:    versionCount,
		VersionsHash:    versionsHash,
		Mode:            ModeDeterministic,
		EffectsAllowed:  false,
	}
	return result, nil
}
