package replay

import (
	app "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// RunMode runs any replay mode with caller-supplied capabilities. Production
// callers use Run, RunRecorded and RunShadow; tests use RunMode to inject
// ledgers and executors.
var RunMode = app.RunMode

// DeterministicBaseline is the in-repository non-model shadow policy.
type DeterministicBaseline = domain.BaselinePolicy

// NewDeterministicBaseline creates the baseline from a compiled spec.
var NewDeterministicBaseline = app.NewDeterministicBaseline
