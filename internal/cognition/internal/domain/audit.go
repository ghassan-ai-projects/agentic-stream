package domain

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const AuditTriggerName = "playbook_audit"

type AuditRequest struct {
	TenantID         string
	SituationID      string
	SituationVersion int
	Key              string
	Reason           string
}

func AuditEvaluation(request AuditRequest, policyDigest string, now time.Time) Evaluation {
	return Evaluation{TriggerID: sources.PrefixTrigger + request.Key, TriggerName: AuditTriggerName, SituationID: request.SituationID,
		SituationVersion: request.SituationVersion, Lane: spec.LaneDeep, Outcome: "admitted", Reasons: []string{request.Reason},
		PolicySHA256: policyDigest, DeltaJSON: []byte("{}"), EvaluatedAt: now}
}

func AuditItem(eval Evaluation, now time.Time) episodeledger.SchedulerItem {
	item := NewSchedulerItem(eval)
	item.ExpiresAt = ReconsiderationExpiry(now)
	return item
}
