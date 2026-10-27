package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) KeyedCases(ctx context.Context, tenantID string) ([]domain.KeyedCase, error) {
	return readAll(ctx, t.tx, "keyed cases", scanKeyedCase, `SELECT diagnosis_code, kind_digest, intent, preset,
		episode_id, verdict, features_json, summary FROM case_verifications WHERE tenant_id = ? AND verdict != 'pending'
		ORDER BY acted_at, situation_id`, tenantID)
}

func scanKeyedCase(rows *sql.Rows) (domain.KeyedCase, error) {
	var c domain.KeyedCase
	var features []byte
	if err := rows.Scan(&c.DiagnosisCode, &c.KindDigest, &c.Intent, &c.Preset, &c.EpisodeID, &c.Verdict, &features, &c.Summary); err != nil {
		return domain.KeyedCase{}, fmt.Errorf("scan settled case: %w", err)
	}
	if err := json.Unmarshal(features, &c.Features); err != nil {
		return domain.KeyedCase{}, fmt.Errorf("decode the features of case %s: %w", c.EpisodeID, err)
	}
	return c, nil
}

const keyClause = `tenant_id = ? AND diagnosis_code = ? AND kind_digest = ? AND intent = ? AND preset = ?`

func keyArgs(tenantID string, key domain.Key) []any {
	return []any{tenantID, key.DiagnosisCode, key.KindDigest, key.Intent, key.Preset}
}

func (t *Tx) KeyHistory(ctx context.Context, tenantID string, key domain.Key) (domain.KeyHistory, error) {
	var history domain.KeyHistory
	var rejected sql.NullInt64
	args := append(keyArgs(tenantID, key), keyArgs(tenantID, key)...)
	args = append(args, keyArgs(tenantID, key)...)
	if err := t.tx.QueryRowContext(ctx, `SELECT
			EXISTS (SELECT 1 FROM rule_drafts WHERE `+keyClause+` AND status = 'requested') OR
			EXISTS (SELECT 1 FROM rule_proposals WHERE `+keyClause+` AND status IN ('pending', 'approved', 'written')),
			(SELECT MAX(evidence_cases) FROM rule_proposals WHERE `+keyClause+` AND status = 'rejected')`, args...).
		Scan(&history.Pending, &rejected); err != nil {
		return history, fmt.Errorf("read the proposal history of %s/%s: %w", key.DiagnosisCode, key.Intent, err)
	}
	history.Rejected, history.RejectedEvidence = rejected.Valid, int(rejected.Int64)
	return history, nil
}

func (t *Tx) RequestDraft(ctx context.Context, draft domain.Draft, now time.Time) error {
	args := append([]any{draft.DraftID}, keyArgs(draft.TenantID, draft.Key)...)
	args = append(args, draft.EvidenceCases, kernel.FormatTime(now), kernel.FormatTime(now))
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO rule_drafts (draft_id, tenant_id, diagnosis_code, kind_digest, intent, preset,
		status, evidence_cases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 'requested', ?, ?, ?)
		ON CONFLICT (draft_id) DO NOTHING`, args...); err != nil {
		return fmt.Errorf("request draft %s: %w", draft.DraftID, err)
	}
	return nil
}

func (t *Tx) NextDraft(ctx context.Context, tenantID string) (domain.Draft, bool, error) {
	drafts, err := storage.QueryAll(ctx, t.tx, "requested drafts", scanDraft, `SELECT draft_id, tenant_id, diagnosis_code, kind_digest,
		intent, preset, evidence_cases, attempts FROM rule_drafts WHERE tenant_id = ? AND status = 'requested'
		ORDER BY created_at, draft_id LIMIT 1`, tenantID)
	if err != nil || len(drafts) == 0 {
		return domain.Draft{}, false, wrapRead("read the next draft", err)
	}
	return drafts[0], true, nil
}

func scanDraft(rows *sql.Rows) (domain.Draft, error) {
	var d domain.Draft
	if err := rows.Scan(&d.DraftID, &d.TenantID, &d.Key.DiagnosisCode, &d.Key.KindDigest, &d.Key.Intent, &d.Key.Preset,
		&d.EvidenceCases, &d.Attempts); err != nil {
		return domain.Draft{}, fmt.Errorf("scan draft: %w", err)
	}
	return d, nil
}

func wrapRead(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}

func (t *Tx) StartDraft(ctx context.Context, draftID string, request draftport.Request, now time.Time) error {
	document, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode the request of draft %s: %w", draftID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `UPDATE rule_drafts SET attempts = attempts + 1, request_json = ?, updated_at = ?
		WHERE draft_id = ? AND status = 'requested'`, document, kernel.FormatTime(now), draftID); err != nil {
		return fmt.Errorf("start draft %s: %w", draftID, err)
	}
	return nil
}

func (t *Tx) FinishDraft(ctx context.Context, draftID, status, reason string, reply draftport.Reply, now time.Time) error {
	document, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("encode the reply of draft %s: %w", draftID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `UPDATE rule_drafts SET status = ?, reason = ?, reply_json = ?, input_tokens = input_tokens + ?,
		output_tokens = output_tokens + ?, cost_microunits = cost_microunits + ?, updated_at = ? WHERE draft_id = ?`,
		status, reason, document, reply.Usage.InputTokens, reply.Usage.OutputTokens, reply.Usage.CostMicrounits,
		kernel.FormatTime(now), draftID); err != nil {
		return fmt.Errorf("finish draft %s: %w", draftID, err)
	}
	return nil
}

func (t *Tx) RecordProposal(ctx context.Context, proposal domain.Proposal, now time.Time) error {
	args, err := proposalColumns(proposal, now)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO rule_proposals (proposal_id, tenant_id, diagnosis_code, kind_digest, intent, preset,
		draft_id, risk_class, rule_json, proposal_digest, replay_json, evidence_cases, reason, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, args...); err != nil {
		return fmt.Errorf("record proposal %s: %w", proposal.ProposalID, err)
	}
	return t.announceProposal(ctx, proposal, now)
}

func proposalColumns(proposal domain.Proposal, now time.Time) ([]any, error) {
	rule, err := json.Marshal(proposal.Rule)
	if err != nil {
		return nil, fmt.Errorf("encode the rule of proposal %s: %w", proposal.ProposalID, err)
	}
	replay, err := json.Marshal(proposal.Replay)
	if err != nil {
		return nil, fmt.Errorf("encode the replay of proposal %s: %w", proposal.ProposalID, err)
	}
	args := append([]any{proposal.ProposalID}, keyArgs(proposal.TenantID, proposal.Key)...)
	return append(args, proposal.DraftID, proposal.RiskClass, rule, proposal.Digest, replay, proposal.EvidenceCases, proposal.Rationale,
		kernel.FormatTime(now), kernel.FormatTime(now)), nil
}

func (t *Tx) announceProposal(ctx context.Context, proposal domain.Proposal, now time.Time) error {
	event, err := domain.ProposedEvent(proposal, notify.SourceForTenant(proposal.TenantID), now)
	if err != nil {
		return fmt.Errorf("announce proposal %s: %w", proposal.ProposalID, err)
	}
	if _, err := notify.Append(ctx, t.tx, event, now); err != nil {
		return fmt.Errorf("announce proposal %s: %w", proposal.ProposalID, err)
	}
	return nil
}

func (t *Tx) Proposal(ctx context.Context, tenantID, proposalID string) (domain.Proposal, error) {
	var p domain.Proposal
	var rule, replay []byte
	err := t.tx.QueryRowContext(ctx, `SELECT proposal_id, tenant_id, draft_id, diagnosis_code, kind_digest, intent, preset, risk_class,
		rule_json, proposal_digest, replay_json, evidence_cases, reason, status FROM rule_proposals WHERE tenant_id = ? AND proposal_id = ?`,
		tenantID, proposalID).Scan(&p.ProposalID, &p.TenantID, &p.DraftID, &p.Key.DiagnosisCode, &p.Key.KindDigest, &p.Key.Intent,
		&p.Key.Preset, &p.RiskClass, &rule, &p.Digest, &replay, &p.EvidenceCases, &p.Rationale, &p.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("%w: %s", domain.ErrProposalNotFound, proposalID)
	}
	if err != nil {
		return p, fmt.Errorf("read proposal %s: %w", proposalID, err)
	}
	return p, decodeProposal(&p, rule, replay)
}

func decodeProposal(p *domain.Proposal, rule, replay []byte) error {
	if err := json.Unmarshal(rule, &p.Rule); err != nil {
		return fmt.Errorf("decode the rule of proposal %s: %w", p.ProposalID, err)
	}
	if err := json.Unmarshal(replay, &p.Replay); err != nil {
		return fmt.Errorf("decode the replay of proposal %s: %w", p.ProposalID, err)
	}
	return nil
}

type Resolution struct {
	ProposalID     string
	Status         string
	Approver       string
	Relay          string
	ApprovalDigest string
	Reason         string
	WrittenDigest  string
}

func (t *Tx) ResolveProposal(ctx context.Context, r Resolution, now time.Time) error {
	result, err := t.tx.ExecContext(ctx, `UPDATE rule_proposals SET status = ?, approver_id = ?, relay_id = ?, approval_digest = ?,
		reason = CASE WHEN ? = '' THEN reason ELSE ? END, written_digest = ?, updated_at = ? WHERE proposal_id = ? AND status = 'pending'`,
		r.Status, r.Approver, r.Relay, r.ApprovalDigest, r.Reason, r.Reason, r.WrittenDigest, kernel.FormatTime(now), r.ProposalID)
	if err != nil {
		return fmt.Errorf("resolve proposal %s: %w", r.ProposalID, err)
	}
	resolved, err := storage.RowsAffected(result)
	if err != nil {
		return fmt.Errorf("resolve proposal %s: %w", r.ProposalID, err)
	}
	if resolved == 0 {
		return fmt.Errorf("%w: %s", domain.ErrProposalResolved, r.ProposalID)
	}
	return nil
}
