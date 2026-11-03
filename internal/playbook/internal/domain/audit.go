package domain

const AuditReason = "playbook audit of a rule-handled case"

type AuditRequest struct {
	TenantID         string
	SituationID      string
	SituationVersion int
	Key              string
	Reason           string
}

func AuditRequestFor(fire Fire) AuditRequest {
	return AuditRequest{TenantID: fire.TenantID, SituationID: fire.SituationID, SituationVersion: fire.SituationVersion,
		Key: AuditKey(fire), Reason: AuditReason + " (rule " + fire.Recommendation.RuleID + ")"}
}
