package wire

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func ParseKindFile(raw []byte) (domain.Kind, error) {
	fields, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("plantwire: kind file: %w", err)
	}
	schema, err := schemaOf(kindFileSchema)
	if err != nil {
		return nil, err
	}
	if err := validateAgainst(kindFileSchema, schema, fields); err != nil {
		return nil, err
	}
	return domain.Kind(fields), nil
}
