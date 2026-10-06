package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type approvalHTTPFixture struct {
	db       *storage.DB
	handler  http.Handler
	pipeline *runtime.Pipeline
	clock    *sources.Virtual
	id       string
}

func openApprovalHTTP(t *testing.T, configure ...func(*runtime.PipelineConfig)) approvalHTTPFixture {
	t.Helper()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	db, intent := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	t.Cleanup(func() { _ = db.Close() })
	service := newTestService(t)
	var request policy.Result
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		request, err = service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: intent, Now: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	clk := sources.NewVirtual(now)
	cfg := runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "tenant", Clock: clk}
	for _, change := range configure {
		change(&cfg)
	}
	pipeline, err := runtime.NewPipeline(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeline.Close() })
	handler := api.WithApprovals(api.NewHealthHandler(nil), api.ApprovalConfig{
		Token: "relay-secret", Relay: "relay-1", ErrorStatus: approvalTestStatus,
		Present: func(ctx context.Context, r api.ApprovalSelection) (any, error) {
			return pipeline.ApprovalForSigning(ctx, policy.ApprovalLookup{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved})
		},
		Resolve: func(ctx context.Context, r api.ApprovalSubmission) (any, error) {
			return pipeline.ResolveApproval(ctx, policy.ApprovalResolution{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved, Signature: r.Signature, Reason: r.Reason, Now: now.Add(-24 * time.Hour), TenantID: "foreign"})
		},
	})
	return approvalHTTPFixture{db: db, handler: handler, pipeline: pipeline, clock: clk, id: request.ApprovalID}
}

func approvalTestStatus(err error) int {
	switch {
	case errors.Is(err, policy.ErrApprovalNotFound):
		return 404
	case errors.Is(err, policy.ErrApprovalUnauthorized):
		return 403
	case errors.Is(err, policy.ErrApprovalResolved):
		return 409
	default:
		return 503
	}
}

func approvalRequest(t *testing.T, h http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer relay-secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func signedApproval(t *testing.T, f approvalHTTPFixture, approved bool) []byte {
	t.Helper()
	decision := "false"
	if approved {
		decision = "true"
	}
	rec := approvalRequest(t, f.handler, "GET", "/v1/approvals/"+f.id+"?approver=operator-1&approved="+decision, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var presentation policy.ApprovalPresentation
	if err := json.Unmarshal(rec.Body.Bytes(), &presentation); err != nil {
		t.Fatal(err)
	}
	if presentation.ID != f.id || len(presentation.Request) == 0 {
		t.Fatal(presentation)
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), presentation.SigningBytes)
	body, err := json.Marshal(map[string]any{"approver_id": "operator-1", "approved": approved, "signature": signature, "reason": "operator reviewed immutable evidence"})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func approvalCounts(t *testing.T, db *storage.DB) (commands, outbox int) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM outbox").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	return
}

func approvalStatus(t *testing.T, f approvalHTTPFixture) string {
	t.Helper()
	var status string
	if err := f.db.QueryRowContext(t.Context(), "SELECT status FROM approvals WHERE approval_id = ?", f.id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}
