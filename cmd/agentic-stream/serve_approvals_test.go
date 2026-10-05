package main

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestServeApprovalConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, token, relay, spec string
		fail                     bool
	}{
		{"disabled", "", "", "", false},
		{"token only", "secret", "", "spec", true},
		{"relay only", "", "relay", "spec", true},
		{"no pipeline", "secret", "relay", "", true},
		{"configured", "secret", "relay", "spec", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", tc.token)
			t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", tc.relay)
			flags := serveFlags{liveFlags: liveFlags{specPath: tc.spec}}
			if err := flags.validateApprovalAccess(); (err != nil) != tc.fail {
				t.Fatal(err)
			}
		})
	}
}

func TestServeMountsApprovalsOnlyForConfiguredPipeline(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		pipeline    *runtime.Pipeline
		status      int
	}{
		{"disabled", "", &runtime.Pipeline{}, 404},
		{"no pipeline", "secret", nil, 404},
		{"mounted", "secret", &runtime.Pipeline{}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", tc.token)
			t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", "relay")
			core := &runtimeCore{pipeline: tc.pipeline}
			rec := httptest.NewRecorder()
			core.approvalHandler(api.NewHealthHandler(nil)).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), "GET", "/v1/approvals/id", nil))
			if rec.Code != tc.status {
				t.Fatal(rec.Code)
			}
		})
	}
}

func TestApprovalErrorResponsesPreserveClassification(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{policy.ErrApprovalNotFound, 404}, {policy.ErrApprovalUnauthorized, 403}, {policy.ErrApprovalResolved, 409}, {errors.New("owner lost"), 503}} {
		if got := approvalErrorStatus(tc.err); got != tc.status {
			t.Fatal(got, tc.status)
		}
	}
}
