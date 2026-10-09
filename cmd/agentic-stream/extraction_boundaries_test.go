package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkerFailureStillStopsWhenFailureChannelIsFull(t *testing.T) {
	t.Parallel()
	workerFailure := errors.New("worker failed")
	priorFailure := errors.New("source failed")
	workerErrors := make(chan error, 1)
	failures := make(chan error, 1)
	workerErrors <- workerFailure
	failures <- priorFailure
	stopped := make(chan struct{})
	done := monitorWorkerRuntimeErrors(t.Context(), workerErrors, failures, func() { close(stopped) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker monitor did not finish")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("full failure channel prevented runtime cancellation")
	}
	if got := <-failures; !errors.Is(got, priorFailure) {
		t.Fatalf("worker monitor replaced prior source failure with %v", got)
	}
}

func TestSocketCancellationRetainsPendingSourceFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	failure := errors.New("source failed before cancellation")
	failures := make(chan error, 1)
	failures <- failure
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	done, err := awaitSocketPoll(ctx, ticker, failures)
	if !done || !errors.Is(err, failure) {
		t.Fatalf("pending source failure lost on cancellation: done=%v error=%v", done, err)
	}
}

func TestServeSourceValidationPrecedesCredentialValidation(t *testing.T) {
	t.Parallel()
	flags := serveFlags{liveFlags: liveFlags{dbPath: "runtime.db", specPath: "spec.yaml", tracePath: "trace.jsonl", traceFormat: "simulator"}, liveSocket: "live.sock"}
	flags.worker.EvidenceKey = "invalid"
	_, err := flags.validate()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected source conflict before credentials, got %v", err)
	}
}
