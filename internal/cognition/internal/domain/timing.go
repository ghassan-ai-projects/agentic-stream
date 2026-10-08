package domain

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	defaultExpiresAfter = 15 * time.Minute
	globalCapacity      = 100
)

func CapacityExhausted(pending, sameTrigger int) bool {
	return pending >= globalCapacity && sameTrigger == 0
}

// applyTiming sets the item's expiry and, when the trigger debounces, the
// earliest time it may run.
func ApplyTiming(item *episodeledger.SchedulerItem, trigger spec.Trigger, now time.Time) error {
	expiresAfter, err := ParseOptionalDuration(trigger.ExpiresAfter, defaultExpiresAfter)
	if err != nil {
		return fmt.Errorf("parse expiresAfter: %w", err)
	}
	item.ExpiresAt = now.Add(expiresAfter)
	debounce, err := ParseOptionalDuration(trigger.Debounce, 0)
	if err != nil {
		return fmt.Errorf("parse debounce: %w", err)
	}
	if debounce > 0 {
		notBefore := now.Add(debounce)
		item.NotBefore = &notBefore
	}
	return nil
}

func ParseOptionalDuration(s string, defaultDur time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultDur, nil
	}
	d, err := spec.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return d, nil
}

func ApplyCooldown(item *episodeledger.SchedulerItem, latest time.Time, cooldown time.Duration) {
	notBefore := latest.Add(cooldown)
	if item.NotBefore == nil || notBefore.After(*item.NotBefore) {
		item.NotBefore = &notBefore
	}
}

func FindTrigger(compiled *spec.CompiledSpec, name string) (spec.Trigger, error) {
	for _, tr := range compiled.Cognition.Triggers {
		if tr.Name == name {
			return tr, nil
		}
	}
	return spec.Trigger{}, fmt.Errorf("trigger %q not found", name)
}

func SchedulerDedupeKey(situationID string, version int, triggerID string) []byte {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s", situationID, version, triggerID)
	return h.Sum(nil)
}

func Defer(evaluation *Evaluation) {
	evaluation.Outcome = "deferred"
	evaluation.Reasons = append(evaluation.Reasons, "global capacity exhausted")
}

func RecordCostRefusal(reasons []string, refusal string) []string {
	return append(reasons, "episode admission rejected by cost control: "+refusal)
}

func ReconsiderationExpiry(now time.Time) time.Time { return now.Add(defaultExpiresAfter) }

func Supersedes(replacement, old int) bool { return old < replacement }

func HasPreviousVersion(version int) bool { return version > 0 }

// NewSchedulerItem binds an admitted evaluation to its pending queue record.
func NewSchedulerItem(id string, eval Evaluation) episodeledger.SchedulerItem {
	return episodeledger.SchedulerItem{SchedulerItemID: id, Kind: episodeledger.KindStandard, TriggerID: eval.TriggerID, SituationID: eval.SituationID, SituationVersion: eval.SituationVersion, Lane: eval.Lane, Priority: eval.Score, Status: "pending"}
}
