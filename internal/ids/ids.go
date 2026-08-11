// Package ids generates stable identifiers for the runtime.
package ids

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"sync/atomic"
)

// Prefixes for the different runtime identity spaces.
const (
	PrefixEvent     = "evt_"
	PrefixSituation = "sit_"
	PrefixEpisode   = "epi_"
	PrefixScheduler = "sch_"
	PrefixTrigger   = "trg_"
	PrefixDecision  = "dec_"
	PrefixIntent    = "int_"
	PrefixCommand   = "cmd_"
	PrefixArtifact  = "art_"
	PrefixReplay    = "rpl_"
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

// PrefixSequence is a deterministic generator that maintains an independent
// sequence per prefix. This keeps IDs short and stable across identity spaces.
type PrefixSequence struct {
	mu  sync.Mutex
	seq map[string]uint64
}

// New returns the next ID for prefix.
func (g *PrefixSequence) New(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seq == nil {
		g.seq = make(map[string]uint64)
	}
	g.seq[prefix]++
	return fmt.Sprintf("%s%016x", prefix, g.seq[prefix])
}

// Reset clears all sequences.
func (g *PrefixSequence) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq = make(map[string]uint64)
}

// Sequence is a convenience wrapper for a single prefix.
type Sequence struct {
	prefix string
	n      atomic.Uint64
}

// NewSequence creates a sequence generator for prefix.
func NewSequence(prefix string) *Sequence {
	return &Sequence{prefix: prefix}
}

// New returns the next value in the sequence.
func (s *Sequence) New() string {
	return fmt.Sprintf("%s%016x", s.prefix, s.n.Add(1))
}
