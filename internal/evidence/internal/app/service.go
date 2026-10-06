package app

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"time"
)

// CapabilityConfig supplies immutable authority names, key material and clocks.
type CapabilityConfig struct {
	Issuer, Audience, KeyID string
	Keys                    map[string][]byte
	Now                     func() time.Time
	MaxTTL, ClockSkew       time.Duration
}

// CallConfig binds the verified scope and durable call ports.
type CallConfig struct {
	Capabilities, Ledger *Service
	Query                Query
	RuntimeEpoch         string
	Now                  func() time.Time
}

// Config explicitly selects capability, ledger and call responsibilities.
type Config struct {
	Capabilities *CapabilityConfig
	Ledger       *Ledger
	Calls        *CallConfig
}

// Service owns configured evidence use cases without exposing mutable ports.
type Service struct {
	issuer   *Issuer
	verifier *Verifier
	ledger   *Ledger
	server   *Server
}

// New validates each configured responsibility before it is available.
func New(cfg Config) (*Service, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	service := &Service{ledger: cfg.Ledger}
	service.configureCapabilities(cfg.Capabilities)
	service.configureCalls(cfg.Calls)
	return service, nil
}
func validateConfig(cfg Config) error {
	if cfg.Capabilities == nil && cfg.Ledger == nil && cfg.Calls == nil {
		return fmt.Errorf("evidence configuration is required")
	}
	if err := validateCapabilities(cfg.Capabilities); err != nil {
		return err
	}
	if cfg.Ledger != nil && (!cfg.Ledger.Store.Configured() || cfg.Ledger.LeaseOwner == "") {
		return fmt.Errorf("evidence ledger database, owner check, lease owner and runtime epoch are required")
	}
	return validateCalls(cfg.Calls)
}
func validateCapabilities(cfg *CapabilityConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.Issuer == "" || cfg.Audience == "" || cfg.KeyID == "" || len(cfg.Keys[cfg.KeyID]) < 32 {
		return fmt.Errorf("evidence capability issuer, audience and signing key are required")
	}
	if cfg.MaxTTL < 0 || cfg.ClockSkew < 0 {
		return fmt.Errorf("evidence capability lifetime and clock skew must not be negative")
	}
	for _, key := range cfg.Keys {
		if len(key) < 32 {
			return fmt.Errorf("evidence capability key is too short")
		}
	}
	return nil
}
func validateCalls(cfg *CallConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.Capabilities == nil || cfg.Capabilities.verifier == nil || cfg.Ledger == nil || cfg.Ledger.ledger == nil || cfg.Query == nil || cfg.RuntimeEpoch == "" {
		return fmt.Errorf("evidence calls require capabilities, durable ledger, query and runtime epoch")
	}
	if cfg.Ledger.ledger.RuntimeEpoch != cfg.RuntimeEpoch {
		return fmt.Errorf("evidence ledger runtime epoch does not match call epoch")
	}
	return nil
}
func (s *Service) configureCapabilities(cfg *CapabilityConfig) {
	if cfg == nil {
		return
	}
	keys := copyKeys(cfg.Keys)
	s.issuer = &Issuer{Issuer: cfg.Issuer, Audience: cfg.Audience, KeyID: cfg.KeyID, Keys: keys, Now: cfg.Now, MaxTTL: cfg.MaxTTL}
	s.verifier = &Verifier{Issuer: cfg.Issuer, Audience: cfg.Audience, Keys: keys, Now: cfg.Now, MaxTTL: cfg.MaxTTL, ClockSkew: cfg.ClockSkew}
}
func copyKeys(source map[string][]byte) map[string][]byte {
	keys := make(map[string][]byte, len(source))
	for id, key := range source {
		keys[id] = append([]byte(nil), key...)
	}
	return keys
}
func (s *Service) configureCalls(cfg *CallConfig) {
	if cfg != nil {
		s.server = &Server{Verifier: cfg.Capabilities.verifier, Ledger: cfg.Ledger.ledger, Query: cfg.Query, RuntimeEpoch: cfg.RuntimeEpoch, Now: cfg.Now}
	}
}

// Issue signs a capability through the configured issuing port.
func (s *Service) Issue(scope Scope) ([]byte, error) {
	if s.issuer == nil {
		return nil, fmt.Errorf("evidence capability issuance is not configured")
	}
	return s.issuer.Issue(scope)
}

// Verify authenticates a capability and its scope.
func (s *Service) Verify(token []byte) (Scope, error) {
	if s.verifier == nil {
		return Scope{}, fmt.Errorf("evidence capability verification is not configured")
	}
	return s.verifier.Verify(token)
}

// IssueTime provides the configured clock for attempt capability preparation.
func (s *Service) IssueTime() time.Time {
	if s.issuer != nil && s.issuer.Now != nil {
		return s.issuer.Now().UTC()
	}
	return time.Now().UTC()
}

// Call refuses services that were not configured for worker queries.
func (s *Service) Call(ctx context.Context, req domain.Envelope) (QueryResult, error) {
	if s.server == nil {
		return QueryResult{}, domain.Refuse(domain.FailedPrecondition, "evidence service is not configured")
	}
	return s.server.Call(ctx, req)
}

// RuntimeEpoch identifies the immutable ledger owner binding.
func (s *Service) RuntimeEpoch() string {
	if s.ledger == nil {
		return ""
	}
	return s.ledger.RuntimeEpoch
}

// ReclaimExpired interrupts expired calls using the configured ledger.
func (s *Service) ReclaimExpired(ctx context.Context, now time.Time) error {
	return s.ledger.ReclaimExpired(ctx, now)
}

// RecoverTx recovers calls inside an opaque joined caller transaction.
func (s *Service) RecoverTx(ctx context.Context, tx *store.Tx, now time.Time) (int, error) {
	return s.ledger.RecoverTx(ctx, tx, now)
}

// JoinRecovery preserves the configured owner port on the caller's transaction.
func (s *Service) JoinRecovery() store.Store {
	if s.ledger == nil {
		return store.Store{}
	}
	return s.ledger.Store
}
