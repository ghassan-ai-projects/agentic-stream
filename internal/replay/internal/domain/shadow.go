package domain

import (
	"context"
	"time"
)

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
	Request          EpisodeRequest
}

type EpisodeRequest struct {
	ExecutorName    string
	ExecutorVersion string
	ModelPolicy     string
	PromptVersion   string
	SnapshotSHA256  string
	RequestJSON     []byte
}

type ShadowOutput struct {
	ExecutorVersion string
	ManifestSHA256  string
	DecisionJSON    []byte
	DecisionSHA256  string
}

type ShadowExecutor interface {
	ExecuteShadow(context.Context, ShadowInput) (ShadowOutput, error)
}

type BaselineExecutor interface {
	ExecuteBaseline(context.Context, ShadowInput) (ShadowOutput, error)
}

type Capabilities struct {
	RecordedLedger   RecordedLedger
	BaselineExecutor BaselineExecutor
	ShadowExecutor   ShadowExecutor
}

func (i ShadowInput) Clone() ShadowInput {
	i.SnapshotJSON = append([]byte(nil), i.SnapshotJSON...)
	i.Request.RequestJSON = append([]byte(nil), i.Request.RequestJSON...)
	return i
}
