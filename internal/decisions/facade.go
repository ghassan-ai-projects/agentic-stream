package decisions

import "github.com/ghassan-ai-projects/agentic-stream/internal/decisions/internal/domain"

// Input supplies the trusted episode and snapshot authority for validation.
type Input = domain.Input

// Result contains the validated Decision identity and ordered Intents.
type Result = domain.Result

// Intent is one validated, digest-bound action proposal.
type Intent = domain.Intent

// IntentCatalog is opaque compiled authority created by CompileIntentCatalog.
type IntentCatalog = domain.IntentCatalog

// ValidationError describes a fail-closed Decision rejection.
type ValidationError = domain.ValidationError

// Validate delegates Decision and Intent validation to the domain rules.
func Validate(raw []byte, transmittedDigest string, input Input) (*Result, error) {
	return domain.Validate(raw, transmittedDigest, input)
}

// CompileIntentCatalog delegates catalog validation and compilation.
func CompileIntentCatalog(document []map[string]any) (*IntentCatalog, error) {
	return domain.CompileIntentCatalog(document)
}
