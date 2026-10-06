package domain

import "errors"

// ErrLiveEpisodeConflict means a reconsideration collides with a live episode.
var ErrLiveEpisodeConflict = errors.New("one live episode per situation constraint")

// Dispatch policies.
const (
	DispatchShadow = "shadow"
	KindReconsider = "reconsider"
)

// Admission is the materialized durable episode input, independent of an executor.
type Admission struct {
	EpisodeID, SchedulerItemID, Kind, TenantID, SituationID                  string
	SituationVersion                                                         int
	ExecutorName, ExecutorVersion, ModelPolicy, PromptVersion                string
	SnapshotSHA256, PromptSHA256, ObjectiveSHA256, AdmissionKey, RequestJSON []byte
	DispatchPolicy, PolicyEpoch                                              string
}

// EffectiveDispatchPolicy is the declared policy, or shadow when none was
// declared: nothing enters action governance unless the spec declared active.
func (a Admission) EffectiveDispatchPolicy() string {
	if a.DispatchPolicy == "" {
		return DispatchShadow
	}
	return a.DispatchPolicy
}

// ReportsLiveConflict reports whether a live-episode uniqueness violation
// counts as a conflict: only a reconsideration reports one.
func (a Admission) ReportsLiveConflict() bool { return a.Kind == KindReconsider }
