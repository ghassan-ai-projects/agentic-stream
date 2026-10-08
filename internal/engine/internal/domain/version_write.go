package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

// SituationWrite is the current-row content derived from a published version.
type SituationWrite struct {
	OccurrenceID           string
	FirstEventTime         time.Time
	StateJSON, StateDigest []byte
}

// NewSituationWrite requires the version's runtime state and defaults a missing
// occurrence ID and first event time.
func NewSituationWrite(version situations.Version) (SituationWrite, error) {
	if len(version.StateJSON) == 0 {
		return SituationWrite{}, fmt.Errorf("situation %s version %d has no runtime state", version.SituationID, version.Version)
	}
	stateDigest, err := situationStateDigest(version.StateJSON, version.StateSHA256)
	if err != nil {
		return SituationWrite{}, err
	}
	digest, err := DecodeStateDigest(stateDigest)
	if err != nil {
		return SituationWrite{}, err
	}
	return SituationWrite{OccurrenceID: occurrenceIDOf(version), FirstEventTime: firstEventTimeOf(version), StateJSON: version.StateJSON, StateDigest: digest}, nil
}

// DecodeStateDigest decodes a "sha256:" Situation state digest to its bytes.
func DecodeStateDigest(stateDigest string) ([]byte, error) {
	digest, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return nil, fmt.Errorf("invalid situation state digest: %w", err)
	}
	return digest, nil
}

func situationStateDigest(stateJSON []byte, digest string) (string, error) {
	if digest != "" {
		return digest, nil
	}
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		return "", fmt.Errorf("decode situation state for digest: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return "", fmt.Errorf("digest situation state: %w", err)
	}
	return digest, nil
}

func occurrenceIDOf(version situations.Version) string {
	if version.OccurrenceID == "" {
		return "occ-" + version.SituationID
	}
	return version.OccurrenceID
}

func firstEventTimeOf(version situations.Version) time.Time {
	if version.FirstEventTime.IsZero() {
		return version.EventHorizon
	}
	return version.FirstEventTime
}

// PreviousVersion is the version a published version supersedes; absent for a
// first version.
func PreviousVersion(version situations.Version) (int, bool) {
	if version.PreviousVersion < 1 {
		return 0, false
	}
	return version.PreviousVersion, true
}

// Lineage is the evidence reference set one or more versions rest on.
type Lineage struct {
	ID             string
	Digest         []byte
	ReferencesJSON []byte
}

// NewLineage derives the lineage of an ordered evidence set.
func NewLineage(evidence []string) (Lineage, error) {
	referencesJSON, err := json.Marshal(evidence)
	if err != nil {
		return Lineage{}, fmt.Errorf("marshal evidence: %w", err)
	}
	return Lineage{ID: LineageID(evidence), Digest: canonicaljson.Sum(referencesJSON), ReferencesJSON: referencesJSON}, nil
}

// LineageID derives the stable identity of an ordered evidence set. Each event
// ID is length-prefixed before hashing so distinct evidence sets can never
// collide: event IDs are caller-supplied free text, and a plain concatenation
// would map e.g. ["ab","c"] and ["a","bc"] to the same lineage_id, letting the
// insert silently attach the wrong references to a situation version (an
// explainability-invariant break).
func LineageID(evidence []string) string {
	h := sha256.New()
	var lenBuf [8]byte
	for _, id := range evidence {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(id)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(id))
	}
	return "lin_" + hex.EncodeToString(h.Sum(nil))
}

// DecodeSnapshotDigest decodes a version's "sha256:" snapshot digest to its bytes.
func DecodeSnapshotDigest(digest string) ([]byte, error) {
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot digest: %w", err)
	}
	return decoded, nil
}
