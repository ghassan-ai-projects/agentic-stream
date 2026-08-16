package native

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

// BatchResult is one cell's comparator outcome: the request identity the
// benchmark protocol assigns plus the executor's terminal outcome.
type BatchResult struct {
	CellID     string `json:"cell_id"`
	Status     string `json:"status"`
	AttemptID  string `json:"attempt_id,omitempty"`
	Fence      int64  `json:"fence,omitempty"`
	Decision   []byte `json:"decision_json,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
	Cost       uint64 `json:"cost_microunits,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// RunBatch drives the native executor over the benchmark cells. It is the Go
// side of the `go_native_executor` baseline: the same frozen protocol cells,
// executed by a separate architecture, reported as one JSON document the
// harness merges. A failed cell is reported, never replaced (intention-to-treat).
func RunBatch(ctx context.Context, executor episodes.Executor, requests []*episodes.Request, cellIDs []string) ([]BatchResult, error) {
	if executor == nil {
		return nil, fmt.Errorf("native executor is required")
	}
	if len(requests) != len(cellIDs) {
		return nil, fmt.Errorf("request/cell mismatch: %d requests, %d cell ids", len(requests), len(cellIDs))
	}
	results := make([]BatchResult, 0, len(requests))
	for index, request := range requests {
		result := BatchResult{CellID: cellIDs[index]}
		start := time.Now()
		outcome, err := executor.Execute(ctx, request)
		result.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Status = string(episodes.AttemptFailed)
			result.Reasons = append(result.Reasons, fmt.Sprintf("executor_error:%v", err))
		} else if outcome == nil {
			result.Status = string(episodes.AttemptFailed)
			result.Reasons = append(result.Reasons, "nil_outcome")
		} else {
			result.Status = outcome.Status
			result.AttemptID = outcome.AttemptID
			result.Fence = outcome.Fence
			result.Decision = outcome.DecisionJSON
			result.Cost = outcome.CostMicrounits
			result.Reasons = outcome.Reasons
		}
		results = append(results, result)
	}
	return results, nil
}

// RunBatchJSON is the wire form: cells in, a JSON-encoded report out.
func RunBatchJSON(ctx context.Context, executor episodes.Executor, requests []*episodes.Request, cellIDs []string) ([]byte, error) {
	results, err := RunBatch(ctx, executor, requests, cellIDs)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal batch report: %w", err)
	}
	return encoded, nil
}
