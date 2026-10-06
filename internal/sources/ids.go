// Package sources holds the runtime's injected sources of non-determinism:
// time (a physical clock and a virtual one for replay) and identifiers (a random
// generator and a deterministic one for replay and tests). Rules receive them
// as parameters and never read the wall clock or a random source themselves.
package sources

import (
	"crypto/rand"
	"encoding/base64"
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

// Random returns a generator that produces base64url-encoded random IDs.
func Random() Generator {
	return &randomGenerator{}
}

type randomGenerator struct{}

func (randomGenerator) New(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand only fails under catastrophic conditions.
		panic(fmt.Sprintf("ids: crypto/rand failed: %v", err))
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
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
