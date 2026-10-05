package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
)

func (f serveFlags) validateApprovalAccess() error {
	token, relay := os.Getenv("AGENTIC_STREAM_APPROVAL_TOKEN"), os.Getenv("AGENTIC_STREAM_APPROVAL_RELAY")
	if (token == "") != (relay == "") {
		return fmt.Errorf("approval token and relay must be configured together")
	}
	if token != "" && f.specPath == "" {
		return fmt.Errorf("approval endpoints require --spec")
	}
	return nil
}

func (core *runtimeCore) approvalHandler(base http.Handler) http.Handler {
	token := os.Getenv("AGENTIC_STREAM_APPROVAL_TOKEN")
	if token == "" || core.pipeline == nil {
		return base
	}
	return api.WithApprovals(base, api.ApprovalConfig{Present: core.presentApproval, Resolve: core.resolveApproval, ErrorStatus: approvalErrorStatus, Token: token, Relay: os.Getenv("AGENTIC_STREAM_APPROVAL_RELAY")})
}

func (f serveFlags) validateOperatorAccess() (string, error) {
	if err := f.validateApprovalAccess(); err != nil {
		return "", err
	}
	return f.validateSubscriberAccess()
}
