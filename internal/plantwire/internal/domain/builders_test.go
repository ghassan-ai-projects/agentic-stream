package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const contractDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func valveKind() domain.Kind {
	return domain.Kind{
		"kind": "valve",
		"metrics": []any{map[string]any{"name": "opening", "data_type": "number", "unit": "%",
			"eu_range": map[string]any{"min": json.Number("0"), "max": json.Number("1e2")}}},
		"commands": []any{map[string]any{"name": "set_opening", "risk_class": "R1", "safe_stop": false}},
	}
}

func announced(t *testing.T, kind domain.Kind) domain.Kind {
	t.Helper()
	digest, err := domain.KindDigest(kind)
	if err != nil {
		t.Fatalf("kind digest: %v", err)
	}
	entry := domain.Kind{"kind_digest": digest}
	for member, value := range kind {
		entry[member] = value
	}
	return entry
}

func birthOf(t *testing.T, change func(*domain.Birth)) domain.Birth {
	t.Helper()
	birth := domain.Birth{
		Envelope:        domain.Envelope{Type: domain.TypeNodeBirth, ProtocolVersion: 1, SessionID: "s-1", Lane: "control"},
		ProtocolMin:     1,
		ProtocolMax:     1,
		ContractVersion: domain.ContractVersion,
		ContractDigest:  contractDigest,
		Environment:     domain.EnvironmentSimulated,
		PriorLastSeq:    -1,
		Kinds:           []domain.Kind{announced(t, valveKind())},
		Boards:          []domain.Board{{Board: "board-01", Environment: domain.EnvironmentSimulated, Members: []string{"V1"}}},
	}
	if change != nil {
		change(&birth)
	}
	return birth
}

func boards(environments ...string) []domain.Board {
	listed := make([]domain.Board, len(environments))
	for i, environment := range environments {
		listed[i] = domain.Board{Board: "board-" + environment, Environment: environment, Members: []string{"V1"}}
	}
	return listed
}
