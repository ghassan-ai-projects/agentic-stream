package domain

import (
	"errors"
	"fmt"
)

// ErrFixtureRejected is returned when a production route (no demo mode)
// admits a scheduler item whose executor is `fixture`.
var ErrFixtureRejected = errors.New("fixture executor rejected")

// FixtureExecutor names the demo and test executor.
const FixtureExecutor = "fixture"

// ReconsiderKind is the scheduler item kind of a reconsideration.
const ReconsiderKind = "reconsider"

// AdmissionAttempt identifies the request an admission assembled, so a refusal
// can be reported against it.
type AdmissionAttempt struct {
	Kind        string
	SituationID string
}

// RefuseFixture rejects the `fixture` executor on a production route: it
// exists for demos and tests only, never on a live route.
func RefuseFixture(demoMode bool, executorName string) error {
	if !demoMode && executorName == FixtureExecutor {
		return fmt.Errorf("%w: fixture executor %s on a production route", ErrFixtureRejected, executorName)
	}
	return nil
}
