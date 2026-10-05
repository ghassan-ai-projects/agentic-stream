package replay

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const situationVersionDigestsQuery = `
		SELECT situation_id, version, snapshot_sha256
		FROM situation_versions
		WHERE situation_id IN (
			SELECT situation_id FROM situations WHERE deployment_id = ?
		)
		ORDER BY situation_id, version`

func traceEpoch(ctx context.Context, path, tenantID string, log *eventlog.EventLog) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("open trace: %w", err)
	}
	defer func() { _ = file.Close() }()
	return scanTraceEpoch(ctx, bufio.NewScanner(file), tenantID, log)
}

func scanTraceEpoch(ctx context.Context, scanner *bufio.Scanner, tenantID string, log *eventlog.EventLog) (time.Time, error) {
	var earliest time.Time
	for scanner.Scan() {
		processingTime, valid := traceProcessingTime(ctx, scanner.Bytes(), tenantID, log)
		if !valid {
			continue
		}
		if earliest.IsZero() || processingTime.Before(earliest) {
			earliest = processingTime.UTC()
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("scan trace: %w", err)
	}
	return domain.EpochFromEarliest(earliest), nil
}

// traceProcessingTime shares ingress validity before a line can affect the clock.
func traceProcessingTime(ctx context.Context, line []byte, tenantID string, log *eventlog.EventLog) (time.Time, bool) {
	envelope, ok := domain.TraceEnvelope(line)
	if !ok {
		// JSONLReplay owns malformed-line quarantine. Epoch derivation is
		// only a clock bootstrap and must not turn a quarantinable line into
		// a whole-replay failure.
		return time.Time{}, false
	}
	if !validTraceEnvelope(ctx, &envelope, tenantID, log) {
		return time.Time{}, false
	}
	return domain.EnvelopeProcessingTime(envelope)
}

func validTraceEnvelope(ctx context.Context, envelope *contractsv1.Envelope, tenantID string, log *eventlog.EventLog) bool {
	*envelope = domain.AdoptTenant(*envelope, tenantID)
	if !domain.ContractValidEnvelope(*envelope, tenantID) {
		return false
	}
	if err := log.ValidateEnvelope(ctx, *envelope); err != nil {
		return false
	}
	return true
}

func hashSituationVersions(ctx context.Context, db *storage.DB, deploymentID string) (string, int, error) {
	rows, err := db.QueryContext(ctx, situationVersionDigestsQuery, deploymentID)
	if err != nil {
		return "", 0, fmt.Errorf("query versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	versions, err := collectVersionDigests(rows)
	if err != nil {
		return "", 0, err
	}
	return domain.HashVersionDigests(versions), len(versions), nil
}

func collectVersionDigests(rows *sql.Rows) ([]domain.VersionDigest, error) {
	var versions []domain.VersionDigest
	for rows.Next() {
		var r domain.VersionDigest
		if err := rows.Scan(&r.SituationID, &r.Version, &r.SHA256); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}

	return versions, nil
}

// RunNTimes replays the same trace n times against fresh isolated databases
// and returns the canonical versions hash from each run. All hashes must be
// identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, specPath, tracePath, tenantID string, n int) ([]Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	dir, err := os.MkdirTemp("", "agentic-stream-replay-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	return repeatReplay(ctx, dir, specPath, tracePath, tenantID, n)
}

func repeatReplay(ctx context.Context, dir, specPath, tracePath, tenantID string, n int) ([]Result, error) {
	results := make([]Result, n)
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

// AllHashesEqual reports whether every result has the same VersionsHash.
func AllHashesEqual(results []Result) bool {
	return domain.AllHashesEqual(results)
}
