package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

type EvidenceScope struct {
	EntityID string
	MaxRows  uint64
	MaxBytes uint64
	Window   time.Duration
}

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

func (s EvidenceScope) Query(raw json.RawMessage, horizon time.Time) (EvidenceQuery, error) {
	var args evidenceArguments
	if err := json.Unmarshal(raw, &args); err != nil {
		return EvidenceQuery{}, fmt.Errorf("decode evidence arguments: %w", err)
	}
	if args.EntityID != "" && args.EntityID != s.EntityID {
		return EvidenceQuery{}, fmt.Errorf("evidence entity is outside episode scope")
	}
	return s.scopedQuery(args, horizon)
}

func (s EvidenceScope) scopedQuery(args evidenceArguments, horizon time.Time) (EvidenceQuery, error) {
	query := EvidenceQuery{From: horizon.Add(-s.Window), Until: horizon, MaxRows: s.MaxRows, MaxBytes: s.MaxBytes}
	if args.MaxRows > 0 && args.MaxRows < query.MaxRows {
		query.MaxRows = args.MaxRows
	}
	if args.MaxBytes > 0 && args.MaxBytes < query.MaxBytes {
		query.MaxBytes = args.MaxBytes
	}
	return evidenceWindow(query, args)
}

func evidenceWindow(query EvidenceQuery, args evidenceArguments) (EvidenceQuery, error) {
	from, err := requestedBound(args.From, query.From)
	if err != nil {
		return EvidenceQuery{}, fmt.Errorf("invalid evidence from: %w", err)
	}
	until, err := requestedBound(args.Until, query.Until)
	if err != nil {
		return EvidenceQuery{}, fmt.Errorf("invalid evidence until: %w", err)
	}
	return narrowWindow(query, from, until)
}

func narrowWindow(query EvidenceQuery, from, until time.Time) (EvidenceQuery, error) {
	if from.After(query.From) {
		query.From = from
	}
	if until.Before(query.Until) {
		query.Until = until
	}
	if !query.Until.After(query.From) {
		return EvidenceQuery{}, fmt.Errorf("evidence until must be after from")
	}
	return query, nil
}

func requestedBound(text string, granted time.Time) (time.Time, error) {
	if text == "" {
		return granted, nil
	}
	bound, err := kernel.ParseTime(text)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse evidence bound: %w", err)
	}
	return bound, nil
}
