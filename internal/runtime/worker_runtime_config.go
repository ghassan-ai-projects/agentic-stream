package runtime

import (
	composition "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/composition"
)

// ValidateWorkerRuntimeConfig rejects inconsistent options before opening resources.
func ValidateWorkerRuntimeConfig(cfg WorkerRuntimeConfig) error {
	return composition.ValidateWorkerRuntimeConfig(cfg)
}
