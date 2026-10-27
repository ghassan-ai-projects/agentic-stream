package app

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Config struct {
	Store            store.Store
	Spec             *spec.CompiledSpec
	Playbook         *spec.CompiledPlaybook
	PlaybookPath     string
	TenantID         string
	Clock            sources.Clock
	QueueAudit       store.AuditQueue
	KindDigests      func() map[string]string
	EntityKindDigest func(entityID string) string
	Drafter          draftport.Drafter
	DraftAfter       int
}

type Service struct {
	rules       *domain.Rules
	store       store.Store
	tenantID    string
	clock       sources.Clock
	queue       store.AuditQueue
	kinds       func() map[string]string
	entityKinds func(entityID string) string
	drafter     draftport.Drafter
	draftAfter  int
	file        *playbookFile
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
		queue: cfg.QueueAudit, kinds: cfg.KindDigests, entityKinds: cfg.EntityKindDigest, drafter: cfg.Drafter,
		draftAfter: cmp.Or(cfg.DraftAfter, domain.DefaultDraftAfter), file: loadedFile(cfg)}, nil
}

func loadedFile(cfg Config) *playbookFile {
	if cfg.PlaybookPath == "" || cfg.Playbook == nil {
		return nil
	}
	return &playbookFile{path: cfg.PlaybookPath, spec: cfg.Spec, expected: cfg.Playbook.Digest}
}

func (s *Service) Digest() string { return s.rules.Digest() }

func (s *Service) Store() store.Store { return s.store }
