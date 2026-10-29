package domain

import "errors"

var ErrLiveEpisodeConflict = errors.New("one live episode per situation constraint")

var ErrLiveEpisodeBusy = errors.New("the situation already has a live episode")

const (
	KindStandard   = "standard"
	KindReconsider = "reconsider"
)

type Admission struct {
	EpisodeID, SchedulerItemID, Kind, TenantID, SituationID                  string
	SituationVersion                                                         int
	ExecutorName, ExecutorVersion, ModelPolicy, PromptVersion                string
	SnapshotSHA256, PromptSHA256, ObjectiveSHA256, AdmissionKey, RequestJSON []byte
	DispatchPolicy, PolicyEpoch                                              string
}

func (a Admission) ReportsLiveConflict() bool { return a.Kind == KindReconsider }

type DispatchableEpisode struct {
	Admission
	StaleRebindCount int
}
