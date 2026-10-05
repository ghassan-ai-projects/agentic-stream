package domain

// Result is the durable policy result for one Intent evaluation.
type Result struct {
	IntentID   string
	DecisionID string
	Result     string
	Reason     string
	CommandID  string
	ApprovalID string
}

// WithReason returns a copy with Reason set (used by calibrated automation).
func (r Result) WithReason(reason string) Result {
	r.Reason = reason
	return r
}

// ApprovalAssertion is the signed, single-use approval binding. The runtime
// reconstructs the canonical bytes from durable rows before verifying it.
type ApprovalAssertion struct {
	ApprovalID       string
	IntentID         string
	DecisionID       string
	TenantID         string
	SituationID      string
	SituationVersion int
	RiskClass        string
	IntentDigest     string
	DecisionDigest   string
	ExpiresAt        string
	Nonce            string
	ApproverID       string
	RelayID          string
}

type IntentRecord struct {
	IntentID            string
	DecisionID          string
	EpisodeID           string
	EpisodeTenant       string
	EpisodeSituation    string
	EpisodeVersion      int
	SituationTenant     string
	DecisionSituation   string
	DecisionVersion     int
	TenantID            string
	SituationID         string
	SituationVersion    int
	IntentType          string
	RiskClass           string
	IntentJSON          []byte
	IntentSHA           []byte
	RateLimitPerHour    int
	RequiresApproval    int
	ExpiresAt           string
	PolicyStatus        string
	ValidationStatus    string
	DecisionJSON        []byte
	DecisionSHA         []byte
	Traceparent         string
	Tracestate          string
	EpisodeLifecycle    string
	CurrentSituation    int
	CurrentCompleteness string
	ExecutorVersion     string
	SituationType       string
	PolicyEpoch         string
}
