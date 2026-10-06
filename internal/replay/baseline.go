package replay

import (
	app "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// DeterministicBaseline is the in-repository non-model shadow policy. It
// selects only from the spec's declared intent catalog and emits no action
// plane object. Its simple rule is intentionally transparent: it selects the
// first declared intent and fills only schema-declared, bounded values; if the
// catalog cannot be satisfied it abstains explicitly.
type DeterministicBaseline = domain.BaselinePolicy

// NewDeterministicBaseline creates the baseline from an immutable compiled
// spec. The caller still validates its output through the normal Decision and
// Intent catalog validator.
func NewDeterministicBaseline(compiled *spec.CompiledSpec) (*DeterministicBaseline, error) {
	return app.NewDeterministicBaseline(compiled)
}
