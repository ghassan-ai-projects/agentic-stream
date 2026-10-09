package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

// Maintenance drains committed command outbox work without new sensor input.
func TestACommittedApprovalDispatchesWithoutNewSensorInput(t *testing.T) {
	t.Parallel()
	f := openApprovalHTTP(t, func(cfg *runtime.PipelineConfig) { cfg.MaintenanceInterval = 10 * time.Millisecond })
	body := signedApproval(t, f, true)
	if rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
	}
	if err := f.pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if scalar[string](t, f.db, "SELECT status FROM outbox WHERE kind = 'command'") == "delivered" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("the committed command was not delivered without sensor input")
		case <-ticker.C:
		}
	}
}
