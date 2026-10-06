package domain

import (
	"fmt"
	"sync"
)

// Prefixes for the runtime identity spaces, one per kind of durable record.
const (
	PrefixEvent           = "evt_"
	PrefixSituation       = "sit_"
	PrefixEpisode         = "epi_"
	PrefixAttempt         = "att_"
	PrefixScheduler       = "sch_"
	PrefixTrigger         = "trg_"
	PrefixDecision        = "dec_"
	PrefixIntent          = "int_"
	PrefixApproval        = "apr_"
	PrefixCommand         = "cmd_"
	PrefixOutcome         = "out_"
	PrefixPolicy          = "pol_"
	PrefixVerification    = "ver_"
	PrefixLease           = "lease_"
	PrefixReconsideration = "rec_"
	PrefixAudit           = "aud_"
	PrefixShadow          = "shd_"
)

// Generator produces unique identifiers. It is safe for concurrent use.
type Generator interface {
	// New returns a new identifier with the given prefix.
	New(prefix string) string
}

// Deterministic returns a generator that produces numbered IDs for tests and
// replay. The sequence is global per generator instance and safe for concurrent
// use.
func Deterministic() Generator {
	return &deterministicGenerator{}
}

type deterministicGenerator struct {
	mu  sync.Mutex
	seq uint64
}

func (g *deterministicGenerator) New(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return fmt.Sprintf("%s%016x", prefix, g.seq)
}
