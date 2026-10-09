package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
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
)

type approvalHTTPFixture struct {
	pendingApproval
	handler  http.Handler
	pipeline *runtime.Pipeline
	clock    *sources.Virtual
}

func openApprovalHTTP(t *testing.T, configure ...func(*runtime.PipelineConfig)) approvalHTTPFixture {
	t.Helper()
	p := openPendingApproval(t)
	compiled, err := spec.CompileFile(t.Context(), "../../../../examples/predictive-maintenance/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	clk := sources.NewVirtual(p.now)
	cfg := runtime.PipelineConfig{DB: p.db, Spec: compiled, TenantID: "tenant", Clock: clk}
	for _, change := range configure {
		change(&cfg)
	}
	pipeline, err := runtime.NewPipeline(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeline.Close() })
	handler := api.WithApprovals(http.NotFoundHandler(), api.ApprovalConfig{
		Token: "relay-secret", Relay: "relay-1", ErrorStatus: approvalTestStatus,
		Present: func(ctx context.Context, r api.ApprovalSelection) (any, error) {
			return pipeline.ApprovalForSigning(ctx, policy.ApprovalLookup{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved})
		},
		Resolve: func(ctx context.Context, r api.ApprovalSubmission) (any, error) {
			return pipeline.ResolveApproval(ctx, policy.ApprovalResolution{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved, Signature: r.Signature, Reason: r.Reason, Now: p.now.Add(-24 * time.Hour), TenantID: "foreign"})
		},
	})
	return approvalHTTPFixture{pendingApproval: p, handler: handler, pipeline: pipeline, clock: clk}
}

func approvalTestStatus(err error) int {
	switch {
	case errors.Is(err, policy.ErrApprovalNotFound):
		return http.StatusNotFound
	case errors.Is(err, policy.ErrApprovalUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, policy.ErrApprovalResolved):
		return http.StatusConflict
	default:
		return http.StatusServiceUnavailable
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
	rec := approvalRequest(t, f.handler, "GET", "/v1/approvals/"+f.approvalID+"?approver=operator-1&approved="+decision, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("presentation = %d %s", rec.Code, rec.Body.String())
	}
	var presentation policy.ApprovalPresentation
	if err := json.Unmarshal(rec.Body.Bytes(), &presentation); err != nil {
		t.Fatal(err)
	}
	if presentation.ID != f.approvalID || len(presentation.Request) == 0 {
		t.Fatal(presentation)
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), presentation.SigningBytes)
	body, err := json.Marshal(map[string]any{"approver_id": "operator-1", "approved": approved, "signature": signature, "reason": "operator reviewed immutable evidence"})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
