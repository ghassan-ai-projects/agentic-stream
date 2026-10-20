package app

import (
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func effectOf(order domain.CommandOrder, settlement domain.Settlement, trail []map[string]any) (actionport.Effect, error) {
	effect := actionport.Effect{ProviderResult: providerResult(order, settlement.Phase, settlement.Reason, trail)}
	switch settlement.Phase {
	case domain.SettlementSucceeded:
		effect.VerificationPending = true
		return effect, nil
	case domain.SettlementUnknown:
		return effect, &actionport.UnknownOutcomeError{Err: commandError(order, nil, settlement.Reason)}
	case domain.SettlementFailed:
		return effect, commandError(order, ErrCommandFailed, settlement.Reason)
	case domain.SettlementNotSent:
		return effect, fmt.Errorf("plantwire: command %s not sent: %w", order.CommandID, settlement.Cause)
	}
	return effect, commandError(order, ErrCommandRefused, settlement.Reason)
}

func unknownEffect(order domain.CommandOrder, trail []map[string]any, reason string) (actionport.Effect, error) {
	return effectOf(order, domain.Settlement{CommandID: order.CommandID, Phase: domain.SettlementUnknown, Reason: reason}, trail)
}

func commandError(order domain.CommandOrder, kind error, reason string) error {
	if kind == nil {
		return errors.New("plantwire: command " + order.CommandID + ": " + reason)
	}
	return fmt.Errorf("%w: command %s: %s", kind, order.CommandID, reason)
}

func appendAnswer(trail []map[string]any, settlement domain.Settlement) []map[string]any {
	if settlement.Answer == nil {
		return trail
	}
	return append(trail, settlement.Answer)
}

func providerResult(order domain.CommandOrder, phase, reason string, trail []map[string]any) map[string]any {
	answers := make([]any, len(trail))
	for i, answer := range trail {
		answers[i] = answer
	}
	result := map[string]any{"command_id": order.CommandID, "device": order.Device, "command": order.Command,
		"args": order.Args, "deadline_ns": order.DeadlineNS, "outcome": phase, "answers": answers}
	if reason != "" {
		result["reason"] = reason
	}
	return map[string]any{"plant_wire": result}
}
