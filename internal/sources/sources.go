package sources

import (
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/sources/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources/internal/transport"
)

// Clock provides time to the runtime. The physical clock is safe for concurrent
// use; the virtual clock serializes every method under its own mutex, so it is
// also safe for concurrent use.
type Clock = domain.Clock

// Quality identifies whether a clock is virtualized for replay or backed by
// wall time for live processing.
func Quality(c domain.Clock) string {
	return domain.Quality(c)
}

// Virtual is a deterministic clock for tests and replay. It starts at start
// and advances only when Advance is called. During an Advance, every timer due
// at or before the new time fires in due-time order; timers with the same due
// time fire in the order they were scheduled.
type Virtual = domain.Virtual

// NewVirtual creates a virtual clock with the given start time.
func NewVirtual(start time.Time) *domain.Virtual {
	return domain.NewVirtual(start)
}

const PrefixEvent = domain.PrefixEvent

const PrefixSituation = domain.PrefixSituation

const PrefixEpisode = domain.PrefixEpisode

const PrefixAttempt = domain.PrefixAttempt

const PrefixScheduler = domain.PrefixScheduler

const PrefixTrigger = domain.PrefixTrigger

const PrefixDecision = domain.PrefixDecision

const PrefixIntent = domain.PrefixIntent

const PrefixApproval = domain.PrefixApproval

const PrefixCommand = domain.PrefixCommand

const PrefixOutcome = domain.PrefixOutcome

const PrefixPolicy = domain.PrefixPolicy

const PrefixVerification = domain.PrefixVerification

const PrefixLease = domain.PrefixLease

const PrefixReconsideration = domain.PrefixReconsideration

const PrefixAudit = domain.PrefixAudit

const PrefixShadow = domain.PrefixShadow

// Generator produces unique identifiers. It is safe for concurrent use.
type Generator = domain.Generator

// Deterministic returns a generator that produces numbered IDs for tests and
// replay. The sequence is global per generator instance and safe for concurrent
// use.
func Deterministic() domain.Generator {
	return domain.Deterministic()
}

// Physical returns a clock backed by the operating system, in UTC.
func Physical() Clock { return transport.Physical() }

// Random returns a generator that produces base64url-encoded random identifiers.
func Random() Generator { return transport.Random() }

// OrPhysical returns clock, or the physical clock when clock is nil.
func OrPhysical(clock Clock) Clock {
	if clock == nil {
		return Physical()
	}
	return clock
}

// OrRandom returns generator, or the random generator when generator is nil.
func OrRandom(generator Generator) Generator {
	if generator == nil {
		return Random()
	}
	return generator
}

// DefaultLease is the lease duration used when a caller does not choose one.
const DefaultLease = time.Minute

// OrLease returns lease, or DefaultLease when lease is not positive.
func OrLease(lease time.Duration) time.Duration {
	if lease <= 0 {
		return DefaultLease
	}
	return lease
}
