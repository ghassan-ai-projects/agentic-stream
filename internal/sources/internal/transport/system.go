// Package transport holds the real-world sources: the operating-system clock and
// the cryptographic random identifier generator. They are the only places the
// wall clock and the random generator are read.
package transport

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources/internal/domain"
)

// Physical returns a clock backed by the operating system.
func Physical() domain.Clock {
	return physicalClock{}
}

type physicalClock struct{}

func (physicalClock) Now() time.Time { return time.Now().UTC() }

func (physicalClock) NewTimer(d time.Duration) domain.Timer {
	return physicalTimer{time.NewTimer(d)}
}

type physicalTimer struct {
	t *time.Timer
}

func (t physicalTimer) C() <-chan time.Time { return t.t.C }

func (t physicalTimer) Stop() bool { return t.t.Stop() }

// Random returns a generator that produces base64url-encoded random IDs.
func Random() domain.Generator {
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
