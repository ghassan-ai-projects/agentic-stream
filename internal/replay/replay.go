// Package replay provides deterministic replay of a trace against a spec and
// compares the resulting situation-version hashes for correctness.
package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Result is the deterministic output of a replay run.
type Result struct {
	EventsProcessed int
	VersionCount    int
	VersionsHash    string
}

// Run replays tracePath against specPath and returns the canonical result.
func Run(ctx context.Context, dbPath, specPath, tracePath, tenantID string) (Result, error) {
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return Result{}, fmt.Errorf("compile spec: %w", err)
	}

	log := eventlog.NewEventLog(db)
	conn := ingress.NewJSONLReplay(db, log, tenantID, tracePath, "replay:"+tracePath)
	if _, err := conn.Run(ctx); err != nil {
		return Result{}, fmt.Errorf("replay trace: %w", err)
	}

	eng, err := engine.NewEngine(db, log, clock.Physical(), compiled, tenantID)
	if err != nil {
		return Result{}, fmt.Errorf("new engine: %w", err)
	}

	processed, err := runAllPartitions(ctx, db, eng, tenantID)
	if err != nil {
		return Result{}, fmt.Errorf("run partitions: %w", err)
	}

	versionsHash, versionCount, err := hashSituationVersions(ctx, db, compiled.Digest)
	if err != nil {
		return Result{}, fmt.Errorf("hash situations: %w", err)
	}

	return Result{
		EventsProcessed: processed,
		VersionCount:    versionCount,
		VersionsHash:    versionsHash,
	}, nil
}

func runAllPartitions(ctx context.Context, db *storage.DB, eng *engine.Engine, tenantID string) (int, error) {
	partitions, err := listPartitions(ctx, db, tenantID)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, pid := range partitions {
		n, err := eng.Run(ctx, pid)
		if err != nil {
			return total, fmt.Errorf("run partition %d: %w", pid, err)
		}
		total += n
	}
	return total, nil
}

func listPartitions(ctx context.Context, db *storage.DB, tenantID string) ([]int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT DISTINCT partition_id FROM event_log WHERE tenant_id = ? ORDER BY partition_id",
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var partitions []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("scan partition: %w", err)
		}
		partitions = append(partitions, pid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partitions: %w", err)
	}
	return partitions, nil
}

func hashSituationVersions(ctx context.Context, db *storage.DB, deploymentID string) (string, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT situation_id, version, snapshot_sha256
		FROM situation_versions
		WHERE situation_id IN (
			SELECT situation_id FROM situations WHERE deployment_id = ?
		)
		ORDER BY situation_id, version`,
		deploymentID,
	)
	if err != nil {
		return "", 0, fmt.Errorf("query versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		situationID string
		version     int
		sha256      []byte
	}
	var versions []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.situationID, &r.version, &r.sha256); err != nil {
			return "", 0, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, r)
	}
	if err := rows.Err(); err != nil {
		return "", 0, fmt.Errorf("iterate versions: %w", err)
	}

	// Deterministic canonical hash over ordered version digests.
	h := sha256.New()
	for _, v := range versions {
		_, _ = fmt.Fprintf(h, "%s%d", v.situationID, v.version)
		_, _ = h.Write(v.sha256)
	}
	return hex.EncodeToString(h.Sum(nil)), len(versions), nil
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
	if len(results) == 0 {
		return true
	}
	first := results[0].VersionsHash
	for _, r := range results[1:] {
		if r.VersionsHash != first {
			return false
		}
	}
	return true
}
