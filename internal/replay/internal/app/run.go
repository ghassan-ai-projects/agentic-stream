package app

import (
	"context"
	"fmt"
	"path/filepath"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
)

// Request identifies one replay session: the isolated database path, the
// compiled spec file, the trace file and the tenant.
type Request = domain.Request

// Run replays the request's trace against its spec deterministically and
// returns the canonical result.
func Run(ctx context.Context, request Request) (domain.Result, error) {
	return runSession(ctx, sessionRequest{request: request})
}

// RunMode executes a replay mode without accepting credentials, effectors,
// or a resolver. Worker-aware modes run their explicit capability phase after
// the deterministic stream replay.
func RunMode(ctx context.Context, mode domain.Mode, request Request, capabilities ...domain.Capabilities) (domain.Result, error) {
	caps, err := domain.AdmitCapabilities(capabilities)
	if err != nil {
		return domain.Result{}, err
	}
	if mode == domain.ModeDeterministic {
		return runDeterministicMode(ctx, mode, request)
	}
	if !domain.WorkerAwareMode(mode) {
		return domain.Result{}, fmt.Errorf("%w: %s", domain.ErrUnsupportedMode, mode)
	}
	if err := caps.Validate(mode); err != nil {
		return domain.Result{Mode: mode, EffectsAllowed: false}, err
	}
	return runCapabilityMode(ctx, mode, request, caps)
}

type sessionRequest struct {
	request          domain.Request
	cognitionEnabled bool
	phase            capabilityPhase
	mode             domain.Mode
	caps             domain.Capabilities
}

func runDeterministicMode(ctx context.Context, mode domain.Mode, request Request) (domain.Result, error) {
	result, err := Run(ctx, request)
	result.Mode = mode
	result.WorkerInvoked = false
	result.EffectsAllowed = false
	return result, err
}

func runCapabilityMode(ctx context.Context, mode domain.Mode, request Request, caps domain.Capabilities) (domain.Result, error) {
	session := sessionRequest{
		request: request, cognitionEnabled: true, mode: mode, caps: caps, phase: applyCapabilities,
	}
	result, err := runSession(ctx, session)
	if err != nil {
		return result, err
	}
	result.Mode = mode
	result.EffectsAllowed = false
	return result, err
}

func runSession(ctx context.Context, request sessionRequest) (domain.Result, error) {
	database, err := transport.OpenIsolatedDatabase(ctx, request.request.DBPath)
	if err != nil {
		return domain.Result{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	session, err := prepareReplaySession(ctx, database, request.request.SpecPath, request.request.TracePath, request.request.TenantID)
	if err != nil {
		return domain.Result{}, err
	}
	return session.execute(ctx, request)
}

func (s *replaySession) execute(ctx context.Context, request sessionRequest) (domain.Result, error) {
	processed, err := s.processTrace(ctx, request.request.TracePath, request.cognitionEnabled)
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

// RunNTimes replays the same request n times against fresh isolated
// databases and returns the canonical result of each run. All versions
// hashes must be identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, request Request, n int) ([]domain.Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	var results []domain.Result
	err := transport.WithRunDirectory(func(dir string) error {
		repeated, err := repeatReplay(ctx, dir, request, n)
		results = repeated
		return err
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func repeatReplay(ctx context.Context, dir string, request Request, n int) ([]domain.Result, error) {
	results := make([]domain.Result, n)
	for i := 0; i < n; i++ {
		repeat := request
		repeat.DBPath = filepath.Join(dir, fmt.Sprintf("replay-%d.db", i))
		res, err := Run(ctx, repeat)
		if err != nil {
			return nil, fmt.Errorf("run %d: %w", i, err)
		}
		results[i] = res
	}
	return results, nil
}
