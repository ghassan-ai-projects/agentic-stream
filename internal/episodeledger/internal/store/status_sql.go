package store

import "github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"

var (
	liveLifecycles     = domain.LifecycleSQL(domain.LifecycleStatus.Live)
	closedLifecycles   = domain.LifecycleSQL(domain.LifecycleStatus.Closed)
	inFlightAttempts   = domain.AttemptSQL(domain.AttemptStatus.InFlight)
	unfinishedAttempts = domain.AttemptSQL(domain.AttemptStatus.Unfinished)
)
