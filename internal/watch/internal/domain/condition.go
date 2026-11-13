package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const InstallRoute = actionport.WatchInstallRoute

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusExpired  = "expired"
)

type Condition struct {
	TenantID, SituationID, Expression, Target string
	ExpiresAt                                 time.Time
	SituationVersion, MaxFires                int
}

type ActiveWatch struct {
	Expression, SituationID, Target string
	LastMatched                     *bool
}

func (w ActiveWatch) RisesTo(matched bool) bool {
	return matched && (w.LastMatched == nil || !*w.LastMatched)
}

func (w ActiveWatch) Changes(matched bool) bool {
	return w.LastMatched == nil || *w.LastMatched != matched
}

type Candidate struct{ WatchID, SituationID string }

func WatchID(command actionport.Command) string {
	if command.IdempotencyKey != "" {
		return command.IdempotencyKey
	}
	return command.CommandID
}

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

func (c Condition) SameAs(stored Condition) error {
	sameExpiry := c.ExpiresAt.Equal(stored.ExpiresAt)
	c.ExpiresAt, stored.ExpiresAt = time.Time{}, time.Time{}
	if !sameExpiry || c != stored {
		return errors.New("watch command idempotency conflict")
	}
	return nil
}

func (w ActiveWatch) InScope(situationID, target string) bool {
	return w.SituationID == situationID && w.Target == target
}

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
