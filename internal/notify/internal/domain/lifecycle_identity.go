package domain

import (
	"strconv"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func ApprovalRequestedEvent(tenantID string, p ApprovalRequested, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "approval.requested:" + p.ApprovalID, TenantID: tenantID, Subject: "approval/" + p.ApprovalID,
		PartitionKey: p.SituationID, Payload: p, At: at, Trace: trace,
	}
}

func ApprovalWithdrawnEvent(tenantID string, p ApprovalWithdrawn, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "approval.withdrawn:" + p.ApprovalID, TenantID: tenantID, Subject: "approval/" + p.ApprovalID,
		PartitionKey: p.SituationID, Payload: p, At: at, Trace: trace,
	}
}

func ApprovalResolvedEvent(tenantID string, p ApprovalResolved, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "approval.resolved:" + p.ApprovalID + ":" + p.Status, TenantID: tenantID, Subject: "approval/" + p.ApprovalID,
		PartitionKey: p.SituationID, Payload: p, At: at, Trace: trace,
	}
}

func CommandDispatchedEvent(tenantID string, p CommandDispatched, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "command.dispatched:" + p.CommandID + ":" + p.OutcomeID, TenantID: tenantID, Subject: "command/" + p.CommandID,
		PartitionKey: p.CommandID, Payload: p, At: at, Trace: trace,
	}
}

func OutcomeRecordedEvent(tenantID string, p OutcomeRecorded, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "outcome.recorded:" + p.OutcomeID, TenantID: tenantID, Subject: "outcome/" + p.OutcomeID,
		PartitionKey: p.CommandID, Payload: p, At: at, Trace: trace,
	}
}

func OutcomeReconciledEvent(tenantID string, p OutcomeReconciled, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "outcome.reconciled:" + p.OutcomeID, TenantID: tenantID, Subject: "outcome/" + p.OutcomeID,
		PartitionKey: p.CommandID, Payload: p, At: at, Trace: trace,
	}
}

func SituationSupersededEvent(tenantID, itemID string, p SituationSuperseded, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	id := "situation.superseded:" + p.SituationID + ":" + strconv.Itoa(p.SupersededVersion) + ":" + strconv.Itoa(p.ReplacementVersion) + ":" + itemID
	return LifecycleEvent{
		ID: id, TenantID: tenantID, Subject: "situation/" + p.SituationID,
		PartitionKey: p.SituationID, Payload: p, At: at, Trace: trace,
	}
}

func ReconsiderationAdmittedEvent(tenantID string, p ReconsiderationAdmitted, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return LifecycleEvent{
		ID: "reconsideration.admitted:" + p.ReconsiderationID, TenantID: tenantID, Subject: "situation/" + p.SituationID,
		PartitionKey: p.SituationID, Payload: p, At: at, Trace: trace,
	}
}
