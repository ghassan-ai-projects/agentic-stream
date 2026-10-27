package app

import (
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Config struct {
	Store       store.Store
	Spec        *spec.CompiledSpec
	Playbook    *spec.CompiledPlaybook
	TenantID    string
	Clock       sources.Clock
	QueueAudit  store.AuditQueue
	KindDigests func() map[string]string
}

type Service struct {
	rules    *domain.Rules
	store    store.Store
	tenantID string
	clock    sources.Clock
	queue    store.AuditQueue
	kinds    func() map[string]string
}

func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() || cfg.Spec == nil || cfg.TenantID == "" {
		return nil, errors.New("playbook database, spec and tenant are required")
	}
	rules, err := domain.NewRules(cfg.Spec, cfg.Playbook)
	if err != nil {
		return nil, fmt.Errorf("compile playbook rules: %w", err)
	}
	return &Service{rules: rules, store: cfg.Store, tenantID: cfg.TenantID, clock: sources.OrPhysical(cfg.Clock),
		queue: cfg.QueueAudit, kinds: cfg.KindDigests}, nil
}

func (s *Service) Digest() string { return s.rules.Digest() }

func (s *Service) Store() store.Store { return s.store }
