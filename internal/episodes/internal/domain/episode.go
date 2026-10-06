package domain

import "time"

// Request is the durable input to an episode executor. Its persistence fields
// map to the episodes table; RequestJSON is the canonical executor input.
type Request struct {
	EpisodeID        string // unique episode identity.
	SchedulerItemID  string // scheduler item that admitted this episode.
	Kind             string // standard or reconsider.
	TenantID         string // tenant owning the situation.
	SituationID      string // situation being reasoned about.
	SituationVersion int    // immutable situation version bound to this episode.
	EntityID         string // entity bound to the immutable situation snapshot.
	ExecutorName     string // executor configured in the active spec.
	ExecutorVersion  string // digest of the active spec.
	ModelPolicy      string // model policy from the spec executor.
	PromptVersion    string // prompt version from the spec executor.
	PromptSHA256     string // content-addressed prompt reference.
	ObjectiveSHA256  string // content-addressed objective.
	SnapshotSHA256   string // deterministic hash of the snapshot subset.
	AttemptID        string // worker attempt identity, set at dispatch.
	Fence            int64  // worker fence, set at dispatch.
	AdmissionKey     []byte // unique 32-byte admission key.
	RequestJSON      []byte // canonical JSON sent to the executor.
	// wallTime is populated once from RequestJSON by WallTimeBudget. Keeping the
	// parsed value on the request lets every execution path share one boundary
	// validation without reparsing durable JSON.
	wallTime          time.Duration
	wallTimeValidated bool
	Traceparent       string
	Tracestate        string
	CancellationKey   string
	SupersessionKey   string
	// P8: the mode matrix. DispatchPolicy is active|shadow (from the spec);
	// PolicyEpoch is the runtime owner epoch the episode was admitted under —
	// set ONCE, never rewritten, so a drained epoch refuses only new admission
	// and only a killed epoch refuses in-flight.
	DispatchPolicy string
	PolicyEpoch    string
}
