package domain

import (
	"context"
	"time"
)

// ShadowInput is the immutable Situation snapshot presented to a shadow
// executor. It contains no credential, resolver, or effector capability.
type ShadowInput struct {
	TenantID         string
	EpisodeKey       string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	TriggerID        string
	AttemptID        string
	Fence            int64
	SnapshotDigest   string
	SpecDigest       string
	PolicyDigest     string
	EvaluationTime   time.Time
	SnapshotJSON     []byte
}

// ShadowOutput is the report-only artifact produced by a shadow executor.
type ShadowOutput struct {
	ExecutorVersion string
	ManifestSHA256  string
	DecisionJSON    []byte
	DecisionSHA256  string
}

// ShadowExecutor may inspect a replay snapshot, but cannot dispatch effects.
type ShadowExecutor interface {
	ExecuteShadow(context.Context, ShadowInput) (ShadowOutput, error)
}

// BaselineExecutor is the deterministic, non-model side of a shadow trial.
// It has the same effect-free input/output boundary as ShadowExecutor but is
// named separately so a trial cannot accidentally compare an executor with
// itself.
type BaselineExecutor interface {
	ExecuteBaseline(context.Context, ShadowInput) (ShadowOutput, error)
}

// Capabilities are explicit, non-credential replay adapters.
type Capabilities struct {
	RecordedLedger   RecordedLedger
	BaselineExecutor BaselineExecutor
	ShadowExecutor   ShadowExecutor
}

// Clone returns the input with a private copy of the snapshot bytes so one
// executor cannot mutate what the other sees.
func (i ShadowInput) Clone() ShadowInput {
	i.SnapshotJSON = append([]byte(nil), i.SnapshotJSON...)
	return i
}
