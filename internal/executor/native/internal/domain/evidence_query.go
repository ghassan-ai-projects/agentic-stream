package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// EvidenceScope is what the trusted episode request grants an evidence tool:
// one entity and upper bounds on rows and bytes. Caller arguments may only
// narrow the bounds and never widen the entity.
type EvidenceScope struct {
	EntityID string
	MaxRows  uint64
	MaxBytes uint64
	Window   time.Duration
}

// EvidenceQuery is a scoped, bounded evidence read.
type EvidenceQuery struct {
	From, Until       time.Time
	MaxRows, MaxBytes uint64
}

type evidenceArguments struct {
	EntityID string `json:"entity_id"`
	From     string `json:"from"`
	Until    string `json:"until"`
	MaxRows  uint64 `json:"max_rows"`
	MaxBytes uint64 `json:"max_bytes"`
}

// Query applies the caller's arguments to the scope. The window defaults to the
// scope's Window before now.
func (s EvidenceScope) Query(raw json.RawMessage, now time.Time) (EvidenceQuery, error) {
	var args evidenceArguments
	if err := json.Unmarshal(raw, &args); err != nil {
		return EvidenceQuery{}, fmt.Errorf("decode evidence arguments: %w", err)
	}
	if args.EntityID != "" && args.EntityID != s.EntityID {
		return EvidenceQuery{}, fmt.Errorf("evidence entity is outside episode scope")
	}
	return s.scopedQuery(args, now)
}

func (s EvidenceScope) scopedQuery(args evidenceArguments, now time.Time) (EvidenceQuery, error) {
	query := EvidenceQuery{From: now.Add(-s.Window), Until: now, MaxRows: s.MaxRows, MaxBytes: s.MaxBytes}
	if args.MaxRows > 0 && args.MaxRows < query.MaxRows {
		query.MaxRows = args.MaxRows
	}
	if args.MaxBytes > 0 && args.MaxBytes < query.MaxBytes {
		query.MaxBytes = args.MaxBytes
	}
	return evidenceWindow(query, args)
}

func evidenceWindow(query EvidenceQuery, args evidenceArguments) (EvidenceQuery, error) {
	var err error
	if args.From != "" {
		if query.From, err = time.Parse(time.RFC3339Nano, args.From); err != nil {
			return EvidenceQuery{}, fmt.Errorf("invalid evidence from: %w", err)
		}
	}
	if args.Until != "" {
		if query.Until, err = time.Parse(time.RFC3339Nano, args.Until); err != nil {
			return EvidenceQuery{}, fmt.Errorf("invalid evidence until: %w", err)
		}
	}
	if !query.Until.After(query.From) {
		return EvidenceQuery{}, fmt.Errorf("evidence until must be after from")
	}
	return query, nil
}
