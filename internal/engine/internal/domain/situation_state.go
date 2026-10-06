package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

// SituationState is the persisted runtime state of one Situation.
type SituationState struct {
	SituationID    string               `json:"situation_id"`
	OccurrenceID   string               `json:"occurrence_id"`
	PartitionID    int                  `json:"partition_id"`
	Version        int                  `json:"version"`
	Facts          map[string]any       `json:"facts"`
	Evidence       []string             `json:"evidence"`
	ConditionStart map[string]time.Time `json:"condition_start"`
	Traceparent    string               `json:"traceparent,omitempty"`
	Tracestate     string               `json:"tracestate,omitempty"`
}

// StoredSituation is one current Situation joined with its current version, as
// read from storage.
type StoredSituation struct {
	SituationID, Type, EntityType, EntityID, OccurrenceID, Phase string
	PartitionID, Version, Severity, StateCodecVersion            int
	FirstEventTime, LatestEventTime, UpdatedAt, Completeness     string
	StateJSON, StateSHA256                                       []byte
	PreviousPhase, Traceparent, Tracestate                       string
	Confidence                                                   float64
}

// Restore rebuilds the in-memory Situation: it parses the lifecycle times,
// verifies the persisted state's codec, digest and identity, and restores fact
// times.
func (r StoredSituation) Restore(tenantID, deploymentID string) (situations.Situation, error) {
	times, err := r.parseTimes()
	if err != nil {
		return situations.Situation{}, err
	}
	state, err := r.decodeState()
	if err != nil {
		return situations.Situation{}, err
	}
	return r.situation(tenantID, deploymentID, times, state), nil
}

// situationTimes are the parsed lifecycle times of a stored Situation.
type situationTimes struct{ first, latest, updated time.Time }

func (r StoredSituation) parseTimes() (situationTimes, error) {
	var times situationTimes
	var err error
	if times.first, err = parseSituationTime("first event time", r.FirstEventTime); err != nil {
		return situationTimes{}, err
	}
	if times.latest, err = parseSituationTime("latest event time", r.LatestEventTime); err != nil {
		return situationTimes{}, err
	}
	if times.updated, err = parseSituationTime("updated time", r.UpdatedAt); err != nil {
		return situationTimes{}, err
	}
	return times, nil
}

func parseSituationTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func (r StoredSituation) situation(tenantID, deploymentID string, times situationTimes, state SituationState) situations.Situation {
	return situations.Situation{
		SituationID: r.SituationID, TenantID: tenantID, DeploymentID: deploymentID, Type: r.Type,
		EntityType: r.EntityType, EntityID: r.EntityID, PartitionID: r.PartitionID,
		OccurrenceID: r.OccurrenceID, Version: r.Version, Phase: r.Phase,
		PreviousPhase: r.PreviousPhase, Severity: r.Severity, Confidence: r.Confidence,
		Completeness: r.Completeness, FirstEventTime: times.first, LatestEventTime: times.latest,
		Facts: state.Facts, Evidence: evidenceSet(state.Evidence), ConditionStart: state.ConditionStart,
		OpenedAt: times.first, UpdatedAt: times.updated, Traceparent: r.Traceparent, Tracestate: r.Tracestate,
	}
}

func (r StoredSituation) decodeState() (SituationState, error) {
	if err := r.checkStateCodec(); err != nil {
		return SituationState{}, err
	}
	if err := verifyStateDigest(r.SituationID, r.StateJSON, r.StateSHA256); err != nil {
		return SituationState{}, err
	}
	state, err := decodeState(r.SituationID, r.StateJSON)
	if err != nil {
		return SituationState{}, err
	}
	if state.SituationID != r.SituationID || state.OccurrenceID != r.OccurrenceID || state.PartitionID != r.PartitionID || state.Version != r.Version {
		return SituationState{}, fmt.Errorf("situation %s persisted state identity mismatch", r.SituationID)
	}
	return state, restoreFactTimes(state.Facts)
}

// checkStateCodec requires codec version 1 and complete state bytes; legacy
// codec 0 state must be rebuilt.
func (r StoredSituation) checkStateCodec() error {
	if r.StateCodecVersion == 0 {
		return fmt.Errorf("situation %s requires rebuild: legacy runtime state has no supported codec", r.SituationID)
	}
	if r.StateCodecVersion != 1 {
		return fmt.Errorf("situation %s has unsupported state codec %d", r.SituationID, r.StateCodecVersion)
	}
	if len(r.StateJSON) == 0 || len(r.StateSHA256) != sha256.Size {
		return fmt.Errorf("situation %s has incomplete persisted state", r.SituationID)
	}
	return nil
}

func verifyStateDigest(situationID string, stateJSON, stateSHA256 []byte) error {
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		return fmt.Errorf("decode situation state document %s: %w", situationID, err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return fmt.Errorf("digest situation state %s: %w", situationID, err)
	}
	decodedDigest, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decodedDigest, stateSHA256) {
		return fmt.Errorf("situation %s persisted state digest mismatch", situationID)
	}
	return nil
}

func decodeState(situationID string, stateJSON []byte) (SituationState, error) {
	var state SituationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return SituationState{}, fmt.Errorf("decode situation state %s: %w", situationID, err)
	}
	if state.Facts == nil {
		state.Facts = make(map[string]any)
	}
	return state, nil
}

func restoreFactTimes(facts map[string]any) error {
	for key, value := range facts {
		if !strings.HasSuffix(key, "_event_time") {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return fmt.Errorf("parse fact time %s: %w", key, err)
		}
		facts[key] = parsed
	}
	return nil
}

func evidenceSet(ids []string) map[string]struct{} {
	evidence := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		evidence[id] = struct{}{}
	}
	return evidence
}
