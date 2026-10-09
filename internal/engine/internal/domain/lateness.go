package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type LateDisposition string

const (
	OnTime                    LateDisposition = ""
	LateCorrected             LateDisposition = "corrected"
	LateHistoryOnly           LateDisposition = "history_only"
	LateDropped               LateDisposition = "dropped"
	LateBeyondAllowedLateness LateDisposition = "beyond_allowed_lateness"
)

type LateEvent struct {
	PartitionID          int
	EventID              string
	Position             int64
	EventTime, Watermark time.Time
	Policy               string
	Disposition          LateDisposition
}

func ClassifyLateness(eventTime, watermark time.Time, policy spec.TimePolicy) (LateDisposition, error) {
	if watermark.IsZero() || !eventTime.Before(watermark) {
		return OnTime, nil
	}
	allowed, err := allowedLateness(policy.AllowedLateness)
	if err != nil {
		return OnTime, err
	}
	if watermark.Sub(eventTime) > allowed {
		return LateBeyondAllowedLateness, nil
	}
	return latePolicyDisposition(policy.LatePolicy), nil
}

func (d LateDisposition) ChangesState() bool {
	return d == OnTime || d == LateCorrected
}

func allowedLateness(text string) (time.Duration, error) {
	allowed, err := optionalDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse allowedLateness: %w", err)
	}
	return allowed, nil
}

func latePolicyDisposition(policy string) LateDisposition {
	switch policy {
	case "history_only":
		return LateHistoryOnly
	case "correct", "correct_and_reconsider":
		return LateCorrected
	default:
		return LateDropped
	}
}
