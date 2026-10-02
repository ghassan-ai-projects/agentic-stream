package telemetry

import (
	"sync/atomic"
)

// ObservePipeline records one completed pipeline batch.
func (r *Runtime) ObservePipeline(report PipelineReport) {
	if r == nil {
		return
	}
	add := func(counter *atomic.Uint64, value int) {
		if value > 0 {
			counter.Add(uint64(value))
		}
	}
	add(&r.eventsIngested, report.EventsIngested)
	add(&r.eventsProcessed, report.EventsProcessed)
	add(&r.episodesAdmitted, report.EpisodesAdmitted)
	add(&r.episodesExecuted, report.EpisodesExecuted)
	add(&r.intentsEvaluated, report.IntentsEvaluated)
	add(&r.commandsDispatched, report.CommandsDispatched)
}

// ObserveFailure increments the bounded pipeline failure counter.
func (r *Runtime) ObserveFailure() {
	if r != nil {
		r.streamFailures.Add(1)
	}
}

// ObserveStaleRejection increments the stale-decision rejection counter.
func (r *Runtime) ObserveStaleRejection() {
	if r != nil {
		r.staleRejections.Add(1)
	}
}

// ObserveStaleRebind increments the stale-episode re-bind recovery counter.
func (r *Runtime) ObserveStaleRebind() {
	if r != nil {
		r.staleRebinds.Add(1)
	}
}

// ObserveRebindFailure increments the failed re-bind counter — a live snapshot
// that did not validate (corruption), distinct from a benign stale rejection.
func (r *Runtime) ObserveRebindFailure() {
	if r != nil {
		r.rebindFailures.Add(1)
	}
}

// ObserveDeviceFrameError increments the device-protocol decode/validation
// error counter.
func (r *Runtime) ObserveDeviceFrameError() {
	if r != nil {
		r.deviceFrameErrors.Add(1)
	}
}

// ObserveDeviceReconnect increments the gateway reconnect counter.
func (r *Runtime) ObserveDeviceReconnect() {
	if r != nil {
		r.deviceReconnects.Add(1)
	}
}

// ObserveActionUnknownOutcome increments the ambiguous-action counter.
func (r *Runtime) ObserveActionUnknownOutcome() {
	if r != nil {
		r.actionUnknownOutcomes.Add(1)
	}
}

// ObserveVerificationPending increments the transport-accepted, independently
// unverified action counter.
func (r *Runtime) ObserveVerificationPending() {
	if r != nil {
		r.verificationPending.Add(1)
	}
}

// ObserveVerificationFailure increments the independent-feedback failure
// counter.
func (r *Runtime) ObserveVerificationFailure() {
	if r != nil {
		r.verificationFailures.Add(1)
	}
}

// ObserveLeaseExpiry increments the action lease-expiry counter.
func (r *Runtime) ObserveLeaseExpiry() {
	if r != nil {
		r.leaseExpiries.Add(1)
	}
}

// ObserveSafeStateEntry records a transition into the device-reported safe
// state.
func (r *Runtime) ObserveSafeStateEntry() {
	if r != nil {
		r.safeStateEntries.Add(1)
	}
}

// ObserveReconciliationBarrier records a boot/restart barrier opening.
func (r *Runtime) ObserveReconciliationBarrier() {
	if r != nil {
		r.reconciliationBarriers.Add(1)
	}
}

// ObserveTargetClaimRejection records a competing target authority.
func (r *Runtime) ObserveTargetClaimRejection() {
	if r != nil {
		r.targetClaimRejections.Add(1)
	}
}

// ObserveSafeStopRequested records a priority safe-stop frame request.
func (r *Runtime) ObserveSafeStopRequested() {
	if r != nil {
		r.safeStopRequests.Add(1)
	}
}

// ObserveSafeStopFailure records a safe-stop transport failure.
func (r *Runtime) ObserveSafeStopFailure() {
	if r != nil {
		r.safeStopFailures.Add(1)
	}
}

// ObserveSafeStopCompleted records a safe-stop receipt.
func (r *Runtime) ObserveSafeStopCompleted() {
	if r != nil {
		r.safeStopCompletions.Add(1)
	}
}

// ObserveLiveLineIngested records one valid normalized line received from a
// live ingress source.
func (r *Runtime) ObserveLiveLineIngested() {
	if r != nil {
		r.liveLinesIngested.Add(1)
	}
}

// ObserveLiveLineRejected records one malformed or invalid live ingress line.
func (r *Runtime) ObserveLiveLineRejected() {
	if r != nil {
		r.liveLinesRejected.Add(1)
	}
}
