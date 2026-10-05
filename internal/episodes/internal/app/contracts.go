package app

import "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"

// Request is the bound executor input.
type Request = domain.Request

// Outcome is an attempt's terminal result.
type Outcome = domain.Outcome

// Executor is the bounded reasoning port.
type Executor = domain.Executor
