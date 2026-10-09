package domain

import (
	"sync/atomic"
)

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

func (r *Runtime) ObserveFailure() {
	if r != nil {
		r.streamFailures.Add(1)
	}
}

func (r *Runtime) ObserveStaleRejection() {
	if r != nil {
		r.staleRejections.Add(1)
	}
}

func (r *Runtime) ObserveStaleRebind() {
	if r != nil {
		r.staleRebinds.Add(1)
	}
}

func (r *Runtime) ObserveRebindFailure() {
	if r != nil {
		r.rebindFailures.Add(1)
	}
}

func (r *Runtime) ObserveDeviceFrameError() {
	if r != nil {
		r.deviceFrameErrors.Add(1)
	}
}

func (r *Runtime) ObserveActionUnknownOutcome() {
	if r != nil {
		r.actionUnknownOutcomes.Add(1)
	}
}

func (r *Runtime) ObserveVerificationPending() {
	if r != nil {
		r.verificationPending.Add(1)
	}
}

func (r *Runtime) ObserveLeaseExpiry() {
	if r != nil {
		r.leaseExpiries.Add(1)
	}
}

func (r *Runtime) ObserveSafeStateEntry() {
	if r != nil {
		r.safeStateEntries.Add(1)
	}
}

func (r *Runtime) ObserveReconciliationBarrier() {
	if r != nil {
		r.reconciliationBarriers.Add(1)
	}
}

func (r *Runtime) ObserveTargetClaimRejection() {
	if r != nil {
		r.targetClaimRejections.Add(1)
	}
}

func (r *Runtime) ObserveSafeStopRequested() {
	if r != nil {
		r.safeStopRequests.Add(1)
	}
}

func (r *Runtime) ObserveSafeStopFailure() {
	if r != nil {
		r.safeStopFailures.Add(1)
	}
}

func (r *Runtime) ObserveSafeStopCompleted() {
	if r != nil {
		r.safeStopCompletions.Add(1)
	}
}

func (r *Runtime) ObserveLiveLineIngested() {
	if r != nil {
		r.liveLinesIngested.Add(1)
	}
}

func (r *Runtime) ObserveLiveLineRejected() {
	if r != nil {
		r.liveLinesRejected.Add(1)
	}
}
