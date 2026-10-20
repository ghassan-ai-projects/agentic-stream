package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const kindDigestMember = "kind_digest"

var (
	ErrKindNotCanonical = errors.New("plantwire: a kind has no canonical form")
	ErrKindDigestInFile = errors.New("plantwire: a kind file carries no kind_digest")
	ErrEmptyRange       = errors.New("plantwire: a metric range needs min below max")
)

var rangeMembers = []string{"eu_range", "instrument_range"}

type Kind map[string]any

func (k Kind) Name() string {
	name, _ := k["kind"].(string)
	return name
}

func (k Kind) DeclaredDigest() string {
	digest, _ := k[kindDigestMember].(string)
	return digest
}

func KindDigest(kind Kind) (string, error) {
	body := maps.Clone(map[string]any(kind))
	delete(body, kindDigestMember)
	canonical, err := canonicaljson.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: kind %q: %w", ErrKindNotCanonical, kind.Name(), err)
	}
	return canonicaljson.ContentDigest(canonical), nil
}

func (k Kind) recomputes() bool {
	digest, err := KindDigest(k)
	return err == nil && digest == k.DeclaredDigest()
}

func foldedName(kind Kind) string {
	return strings.ToLower(kind.Name())
}

func checkKindFile(file Kind) error {
	if _, carried := file[kindDigestMember]; carried {
		return fmt.Errorf("%w: kind %q", ErrKindDigestInFile, file.Name())
	}
	metrics, _ := file["metrics"].([]any)
	for i, metric := range metrics {
		if member, empty := emptyRangeOf(metric); empty {
			return fmt.Errorf("%w: kind %q at /metrics/%d/%s", ErrEmptyRange, file.Name(), i, member)
		}
	}
	return nil
}

func emptyRangeOf(metric any) (string, bool) {
	fields, _ := metric.(map[string]any)
	for _, member := range rangeMembers {
		if emptyRange(fields[member]) {
			return member, true
		}
	}
	return "", false
}

func emptyRange(value any) bool {
	bounds, _ := value.(map[string]any)
	low, lowKnown := numberOf(bounds["min"])
	high, highKnown := numberOf(bounds["max"])
	return lowKnown && highKnown && low >= high
}

func numberOf(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case float64:
		return number, true
	}
	return 0, false
}
