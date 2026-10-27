package domain

import "time"

const (
	RouteModel = "model"
	RouteRule  = "rule"
	RouteAudit = "audit"
)

type Admission struct {
	TenantID         string
	EpisodeID        string
	SchedulerItemID  string
	SituationID      string
	SituationVersion int
	EventTime        time.Time
	Facts            map[string]any
	Evidence         []string
}

type Route struct {
	Kind           string
	Recommendation *Recommendation
}

type Report struct {
	AuditsQueued  int
	Settled       int
	Disagreements int
}
