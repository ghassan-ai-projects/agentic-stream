package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// InstallRoute is the effector route that installs a watch condition.
const InstallRoute = "install_watch_condition"

// Watch states (`watch_conditions.status`).
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusExpired  = "expired"
)

// Condition is the identity-defining content of an installed watch.
type Condition struct {
	TenantID, SituationID, Expression, Target string
	ExpiresAt                                 time.Time
	SituationVersion, MaxFires                int
}

// ActiveWatch is the part of a stored active watch that decides a fire.
type ActiveWatch struct{ Expression, SituationID, Target string }

// Candidate is an active watch scoped to one event target.
type Candidate struct{ WatchID, SituationID string }

// WatchID is the identity of a watch: the command's idempotency key, or its
// command ID when none is set.
func WatchID(command actionport.Command) string {
	if command.IdempotencyKey != "" {
		return command.IdempotencyKey
	}
	return command.CommandID
}

// ConditionFromCommand validates a watch payload in order: a bounded, present
// identity (target, Situation, version, 1-100 fires, expression of at most 4096
// characters), then the expression, then a future expiry.
func ConditionFromCommand(command actionport.Command, now time.Time) (Condition, error) {
	condition := Condition{TenantID: command.TenantID}
	condition.Expression, _ = command.Payload["expression"].(string)
	condition.Target, _ = command.Payload["target"].(string)
	condition.SituationID, _ = command.Payload["situation_id"].(string)
	expiresAt, _ := command.Payload["expires_at"].(string)
	var versionOK, firesOK bool
	condition.SituationVersion, versionOK = integerPayload(command.Payload["situation_version"])
	condition.MaxFires, firesOK = integerPayload(command.Payload["max_fires"])
	if condition.Expression == "" || len(condition.Expression) > 4096 || condition.Target == "" || condition.SituationID == "" ||
		!versionOK || condition.SituationVersion < 1 || !firesOK || condition.MaxFires < 1 || condition.MaxFires > 100 {
		return Condition{}, errors.New("watch condition payload is invalid")
	}
	return condition.withValidatedExpiry(expiresAt, now)
}

func (c Condition) withValidatedExpiry(expiresAt string, now time.Time) (Condition, error) {
	if err := ValidateExpression(c.Expression); err != nil {
		return Condition{}, err
	}
	parsedExpiry, err := kernel.ParseTime(expiresAt)
	if err != nil || !parsedExpiry.After(now) {
		return Condition{}, errors.New("watch condition expiry is invalid")
	}
	c.ExpiresAt = parsedExpiry.UTC()
	return c, nil
}

// SameAs makes a repeated install idempotent: the same watch ID must carry the
// same condition.
func (c Condition) SameAs(stored Condition) error {
	sameExpiry := c.ExpiresAt.Equal(stored.ExpiresAt)
	c.ExpiresAt, stored.ExpiresAt = time.Time{}, time.Time{}
	if !sameExpiry || c != stored {
		return errors.New("watch command idempotency conflict")
	}
	return nil
}

// InScope reports whether an event for the given Situation and target is the
// one this active watch was installed for.
func (w ActiveWatch) InScope(situationID, target string) bool {
	return w.SituationID == situationID && w.Target == target
}

// RequireRoute requires the command to name the watch install route.
func RequireRoute(command actionport.Command) error {
	if command.EffectorRoute != InstallRoute {
		return fmt.Errorf("watch effector does not support route %q", command.EffectorRoute)
	}
	return nil
}

func integerPayload(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int(number))
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
