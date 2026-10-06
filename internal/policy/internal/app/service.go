// Package app orchestrates policy evaluation and human approval use cases.
package app

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// Fences are required lower runtime checks on the current transaction.
type Fences struct{ RuntimeOwner, DecisionEpoch store.Fence }

// Config contains validated dependencies supplied by the policy facade.
type Config struct {
	PolicyVersion, PolicyDigest, OwnerEpoch string
	IDGenerator                             ids.Generator
	Fences                                  Fences
	Interlock                               interlock.Reader
}

// Service implements the deterministic policy use cases.
type Service struct {
	policyVersion, policyDigest, ownerEpoch string
	idGen                                   ids.Generator
	fences                                  Fences
	interlock                               interlock.Reader
}

// New accepts dependencies already validated by the facade.
func New(c Config) *Service {
	return &Service{policyVersion: c.PolicyVersion, policyDigest: c.PolicyDigest, ownerEpoch: c.OwnerEpoch, idGen: c.IDGenerator, fences: c.Fences, interlock: c.Interlock}
}
