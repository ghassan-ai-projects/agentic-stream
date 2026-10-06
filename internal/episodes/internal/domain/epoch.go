package domain

import (
	"errors"
	"fmt"
)

// DecisionEpochRefusal maps the configured fence's result to durable episode reasons.
func DecisionEpochRefusal(err, unbound, killed error) (string, error) {
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, unbound):
		return "epoch_unbound", nil
	case errors.Is(err, killed):
		return "epoch_killed", nil
	default:
		return "", fmt.Errorf("assert decision epoch: %w", err)
	}
}
