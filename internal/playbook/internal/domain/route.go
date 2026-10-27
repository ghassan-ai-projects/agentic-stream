package domain

const (
	RouteModel = "model"
	RouteRule  = "rule"
	RouteAudit = "audit"
	RouteSkip  = "skip"
)

type Admission struct {
	TenantID         string
	EpisodeID        string
	SchedulerItemID  string
	SituationID      string
	SituationVersion int
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
