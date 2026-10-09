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

type timing struct{ expiresAfter, debounce, cooldown time.Duration }

func ApplyTiming(item *episodeledger.SchedulerItem, trigger spec.Trigger, now time.Time, latestAdmitted *time.Time) error {
	window, err := parseTiming(trigger)
	if err != nil {
		return err
	}
	item.NotBefore = window.notBefore(now, latestAdmitted)
	item.ExpiresAt = mayStartAt(now, item.NotBefore).Add(window.expiresAfter)
	return nil
}

type timingField struct {
	name, text string
	fallback   time.Duration
	into       *time.Duration
}

func parseTiming(trigger spec.Trigger) (timing, error) {
	var window timing
	fields := []timingField{
		{"expiresAfter", trigger.ExpiresAfter, defaultExpiresAfter, &window.expiresAfter},
		{"debounce", trigger.Debounce, 0, &window.debounce},
		{"cooldown", trigger.Cooldown, 0, &window.cooldown},
	}
	for _, field := range fields {
		if err := field.parse(); err != nil {
			return timing{}, err
		}
	}
	return window, nil
}

func (f timingField) parse() error {
	duration, err := ParseOptionalDuration(f.text, f.fallback)
	if err != nil {
		return fmt.Errorf("parse %s: %w", f.name, err)
	}
	*f.into = duration
	return nil
}

func (w timing) notBefore(now time.Time, latestAdmitted *time.Time) *time.Time {
	var notBefore *time.Time
	if w.debounce > 0 {
		debounced := now.Add(w.debounce)
		notBefore = &debounced
	}
	if w.cooldown > 0 && latestAdmitted != nil {
		cooled := latestAdmitted.Add(w.cooldown)
		if notBefore == nil || cooled.After(*notBefore) {
			notBefore = &cooled
		}
	}
	return notBefore
}

func mayStartAt(now time.Time, notBefore *time.Time) time.Time {
	if notBefore != nil && notBefore.After(now) {
		return *notBefore
	}
	return now
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

func RecordSchedulerExpiry(reasons []string, reason string) []string {
	return append(reasons, "scheduler item expired: "+reason)
}

func ReconsiderationExpiry(now time.Time) time.Time { return now.Add(defaultExpiresAfter) }

func Supersedes(replacement, old int) bool { return old < replacement }

func HasPreviousVersion(version int) bool { return version > 0 }

// NewSchedulerItem binds an admitted evaluation to its pending queue record.
func NewSchedulerItem(id string, eval Evaluation) episodeledger.SchedulerItem {
	return episodeledger.SchedulerItem{SchedulerItemID: id, Kind: episodeledger.KindStandard, TriggerID: eval.TriggerID, SituationID: eval.SituationID, SituationVersion: eval.SituationVersion, Lane: eval.Lane, Priority: eval.Score, Status: "pending"}
}
