package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

// DecodeEvidenceGetArguments parses the closed v1 tool argument schema.
func DecodeEvidenceGetArguments(raw []byte) (domain.EvidenceGetArguments, error) {
	var arguments domain.EvidenceGetArguments
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil || arguments.EntityID == "" {
		return domain.EvidenceGetArguments{}, fmt.Errorf("decode evidence.get arguments")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.EvidenceGetArguments{}, fmt.Errorf("evidence.get arguments contain trailing data")
	}
	return arguments, nil
}
