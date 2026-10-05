package app

import (
	"context"
	"fmt"
	"path/filepath"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
)

// Run replays tracePath against specPath deterministically and returns the
// canonical result.
func Run(ctx context.Context, dbPath, specPath, tracePath, tenantID string) (domain.Result, error) {
	return runSession(ctx, sessionRequest{dbPath: dbPath, specPath: specPath, tracePath: tracePath, tenantID: tenantID})
}

// RunMode executes a replay mode without accepting credentials, effectors,
// or a resolver. Worker-aware modes run their explicit capability phase after
// the deterministic stream replay.
func RunMode(ctx context.Context, mode domain.Mode, dbPath, specPath, tracePath, tenantID string, capabilities ...domain.Capabilities) (domain.Result, error) {
	caps, err := domain.AdmitCapabilities(capabilities)
	if err != nil {
		return domain.Result{}, err
	}
	if mode == domain.ModeDeterministic {
		return runDeterministicMode(ctx, mode, dbPath, specPath, tracePath, tenantID)
	}
	if !domain.WorkerAwareMode(mode) {
		return domain.Result{}, fmt.Errorf("%w: %s", domain.ErrUnsupportedMode, mode)
	}
	if err := caps.Validate(mode); err != nil {
		return domain.Result{Mode: mode, EffectsAllowed: false}, err
	}
	return runCapabilityMode(ctx, mode, dbPath, specPath, tracePath, tenantID, caps)
}

type sessionRequest struct {
	dbPath, specPath, tracePath, tenantID string
	cognitionEnabled                      bool
	phase                                 capabilityPhase
	mode                                  domain.Mode
	caps                                  domain.Capabilities
}

func runDeterministicMode(ctx context.Context, mode domain.Mode, dbPath, specPath, tracePath, tenantID string) (domain.Result, error) {
	result, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
	result.Mode = mode
	result.WorkerInvoked = false
	result.EffectsAllowed = false
	return result, err
}

func runCapabilityMode(ctx context.Context, mode domain.Mode, dbPath, specPath, tracePath, tenantID string, caps domain.Capabilities) (domain.Result, error) {
	request := sessionRequest{
		dbPath: dbPath, specPath: specPath, tracePath: tracePath, tenantID: tenantID,
		cognitionEnabled: true, mode: mode, caps: caps, phase: applyCapabilities,
	}
	result, err := runSession(ctx, request)
	if err != nil {
		return result, err
	}
	result.Mode = mode
	result.EffectsAllowed = false
	return result, err
}

func runSession(ctx context.Context, request sessionRequest) (domain.Result, error) {
	database, err := transport.OpenIsolatedDatabase(ctx, request.dbPath)
	if err != nil {
		return domain.Result{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	session, err := prepareReplaySession(ctx, database, request.specPath, request.tracePath, request.tenantID)
	if err != nil {
		return domain.Result{}, err
	}
	return session.execute(ctx, request)
}

func (s *replaySession) execute(ctx context.Context, request sessionRequest) (domain.Result, error) {
	processed, err := s.processTrace(ctx, request.tracePath, request.cognitionEnabled)
	if err != nil {
		return domain.Result{}, err
	}
	return s.completeReplay(ctx, request, processed)
}

func (s *replaySession) completeReplay(ctx context.Context, request sessionRequest, processed int) (domain.Result, error) {
	result, err := s.collectResult(ctx, processed)
	if err != nil {
		return domain.Result{}, err
	}
	if request.phase != nil {
		if err := request.phase(ctx, s, request.mode, request.caps, s.clk.Now(), &result); err != nil {
			return domain.Result{}, err
		}
	}
	return result, nil
}

// RunNTimes replays the same trace n times against fresh isolated databases
// and returns the canonical result of each run. All versions hashes must be
// identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, specPath, tracePath, tenantID string, n int) ([]domain.Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	var results []domain.Result
	err := transport.WithRunDirectory(func(dir string) error {
		repeated, err := repeatReplay(ctx, dir, specPath, tracePath, tenantID, n)
		results = repeated
		return err
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func repeatReplay(ctx context.Context, dir, specPath, tracePath, tenantID string, n int) ([]domain.Result, error) {
	results := make([]domain.Result, n)
	for i := 0; i < n; i++ {
		dbPath := filepath.Join(dir, fmt.Sprintf("replay-%d.db", i))
		res, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
		if err != nil {
			return nil, fmt.Errorf("run %d: %w", i, err)
		}
		results[i] = res
	}
	return results, nil
}
