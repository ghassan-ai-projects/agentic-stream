package composition

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

func composeAdmissionPlane(cfg PipelineConfig, policyGateway *policy.Service, episodeService *episodes.Service) (*store.PipelineStore, *app.Admitter, error) {
	transactions, err := composeTransactions(cfg, policyGateway, episodeService)
	if err != nil {
		return nil, nil, err
	}
	admitter, err := composeAdmission(cfg, transactions)
	return transactions, admitter, err
}

func composeTransactions(cfg PipelineConfig, policyGateway *policy.Service, episodeService *episodes.Service) (*store.PipelineStore, error) {
	rules, err := composePlaybook(cfg)
	if err != nil {
		return nil, err
	}
	return &store.PipelineStore{DB: cfg.DB, Policy: policyGateway, RuntimeOwner: runtimeOwnershipCheck(cfg), OwnerEpoch: cfg.OwnerEpoch,
		Episodes: episodeService, Playbook: rules, TenantID: cfg.TenantID}, nil
}

func composePlaybook(cfg PipelineConfig) (*playbook.Service, error) {
	if cfg.Playbook == nil {
		return nil, nil
	}
	audits, err := cognition.New(cognition.Config{DeploymentID: cfg.Spec.Digest, TenantID: cfg.TenantID, Spec: cfg.Spec, Clock: cfg.Clock})
	if err != nil {
		return nil, fmt.Errorf("compose playbook audits: %w", err)
	}
	service, err := playbook.New(playbook.Config{DB: cfg.DB, Spec: cfg.Spec, Playbook: cfg.Playbook, TenantID: cfg.TenantID,
		Clock: cfg.Clock, QueueAudit: auditQueue(audits), KindDigests: cfg.KindDigests,
		EntityKind: cfg.EntityKind, PlaybookPath: cfg.PlaybookPath, Drafter: cfg.Drafter, DraftAfter: cfg.DraftAfter,
		Signatures: policySignatures{}})
	if err != nil {
		return nil, fmt.Errorf("compose playbook: %w", err)
	}
	return service, nil
}

func auditQueue(audits *cognition.Service) playbook.AuditQueue {
	return func(ctx context.Context, tx *sql.Tx, request playbook.AuditRequest) (string, error) {
		itemID, err := audits.QueueAudit(ctx, tx, cognition.AuditRequest(request))
		if err != nil {
			return "", fmt.Errorf("queue playbook audit: %w", err)
		}
		return itemID, nil
	}
}

type policySignatures struct{}

func (policySignatures) CheckSigner(ctx context.Context, tx *sql.Tx, approval playbook.SignedApproval) error {
	if err := policy.CheckApprovalSigner(ctx, tx, policy.SignedApproval(approval)); err != nil {
		return fmt.Errorf("check rule approval signer: %w", err)
	}
	return nil
}

func (policySignatures) Verify(ctx context.Context, tx *sql.Tx, approval playbook.SignedApproval) (string, error) {
	digest, err := policy.VerifySignedApproval(ctx, tx, policy.SignedApproval(approval))
	if err != nil {
		return "", fmt.Errorf("verify rule approval: %w", err)
	}
	return digest, nil
}
