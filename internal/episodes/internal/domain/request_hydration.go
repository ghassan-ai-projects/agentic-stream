package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type persistedRequestTrace struct {
	Traceparent     string `json:"traceparent"`
	Tracestate      string `json:"tracestate"`
	CancellationKey string `json:"cancellation_key"`
	SupersessionKey string `json:"supersession_key"`
}

// HydratePersistedRequest restores and validates the request fields that are
// stored only inside request_json.
func HydratePersistedRequest(req *Request) error {
	var trace persistedRequestTrace
	if err := json.Unmarshal(req.RequestJSON, &trace); err != nil {
		return fmt.Errorf("decode persisted request trace context: %w", err)
	}
	if _, err := contractsv1.ParseTraceContext(trace.Traceparent, trace.Tracestate); err != nil {
		return fmt.Errorf("validate persisted request trace context: %w", err)
	}
	req.Traceparent = trace.Traceparent
	req.Tracestate = trace.Tracestate
	req.CancellationKey = trace.CancellationKey
	req.SupersessionKey = trace.SupersessionKey
	return hydrateRequestBudgetEntity(req)
}

func hydrateRequestBudgetEntity(req *Request) error {

	if _, err := req.WallTimeBudget(); err != nil {
		return fmt.Errorf("validate persisted episode budget: %w", err)
	}
	entityID, err := requestEntityID(req.RequestJSON)
	if err != nil {
		return fmt.Errorf("load persisted request entity: %w", err)
	}
	req.EntityID = entityID
	return nil
}

func requestEntityID(raw []byte) (string, error) {
	var request struct {
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return "", fmt.Errorf("decode request snapshot: %w", err)
	}
	if len(request.Snapshot) == 0 {
		return "", nil
	}
	return requestSnapshotEntity(request.Snapshot)
}

func requestSnapshotEntity(raw []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", fmt.Errorf("decode request snapshot entity: %w", err)
	}
	return snapshot.Entity.ID, nil
}
