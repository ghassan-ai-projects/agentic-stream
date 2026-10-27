package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	DraftRequested  = "requested"
	DraftProposed   = "proposed"
	DraftNone       = "no_proposal"
	DraftRefused    = "refused"
	DraftFailed     = "failed"
	maxDraftAttempt = 3

	ProposalPending  = "pending"
	ProposalApproved = "approved"
	ProposalRejected = "rejected"
	ProposalWritten  = "written"

	proposalPrefix     = "rprop_"
	draftPrefix        = "rdraft_"
	ruleApprovalDomain = "situation-runtime/rule-approval/v1"
	proposedEventType  = "playbook.rule.proposed"
	proposedSchema     = "urn:situation-runtime:schema:rule-proposal:v1"
)

var (
	ErrProposalNotFound = errors.New("rule proposal not found")
	ErrProposalResolved = errors.New("rule proposal already resolved")
)

type Draft struct {
	DraftID       string
	TenantID      string
	Key           Key
	EvidenceCases int
	Attempts      int
}

func NewDraft(tenantID string, key Key, evidenceCases int) Draft {
	sum := sha256.Sum256([]byte(key.DiagnosisCode + "\x00" + key.KindDigest + "\x00" + key.Intent + "\x00" + key.Preset + "\x00" + strconv.Itoa(evidenceCases)))
	return Draft{DraftID: draftPrefix + hex.EncodeToString(sum[:12]), TenantID: tenantID, Key: key, EvidenceCases: evidenceCases}
}

func (d Draft) Exhausted() bool { return d.Attempts >= maxDraftAttempt }

type ReplayReport struct {
	VersionsExamined  int `json:"versions_examined"`
	WouldFire         int `json:"would_fire"`
	Decided           int `json:"decided"`
	Agree             int `json:"agree"`
	Disagree          int `json:"disagree"`
	SourceCases       int `json:"source_cases"`
	SourceNotVerified int `json:"source_not_verified"`
}

type ReplayVersion struct {
	Facts    map[string]any
	Evidence []string
	Decision map[string]any
}

type Proposal struct {
	ProposalID    string       `json:"proposal_id"`
	TenantID      string       `json:"tenant_id"`
	DraftID       string       `json:"draft_id"`
	Key           Key          `json:"key"`
	RiskClass     string       `json:"risk_class"`
	Rule          spec.Rule    `json:"rule"`
	Replay        ReplayReport `json:"replay"`
	EvidenceCases int          `json:"evidence_cases"`
	Rationale     string       `json:"rationale"`
	Digest        string       `json:"-"`
	Status        string       `json:"-"`
}

func NewProposal(draft Draft, rule spec.Rule, risk string, replay ReplayReport, proposal draftport.Proposal) (Proposal, error) {
	built := Proposal{ProposalID: proposalPrefix + strings.TrimPrefix(draft.DraftID, draftPrefix), TenantID: draft.TenantID,
		DraftID: draft.DraftID, Key: draft.Key, RiskClass: risk, Rule: rule, Replay: replay, EvidenceCases: draft.EvidenceCases,
		Rationale: proposal.Rationale, Status: ProposalPending}
	digest, err := canonicaljson.Digest(canonicaljson.DomainPlaybook, built)
	if err != nil {
		return Proposal{}, fmt.Errorf("digest proposal %s: %w", built.ProposalID, err)
	}
	built.Digest = digest
	return built, nil
}

func IsProposalID(id string) bool { return strings.HasPrefix(id, proposalPrefix) }

func (r *Rules) Replay(rule spec.Rule, versions []ReplayVersion, source []KeyedCase) (ReplayReport, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return ReplayReport{}, fmt.Errorf("create rule condition environment: %w", err)
	}
	program, err := conditionProgram(env, rule)
	if err != nil {
		return ReplayReport{}, err
	}
	report := sourceRates(Key{DiagnosisCode: rule.DiagnosisCode, KindDigest: rule.PinnedKind.Digest, Intent: rule.Recommend.Intent, Preset: rule.Recommend.Preset}, source)
	recommended := Recommendation{DiagnosisCode: rule.DiagnosisCode, Intent: rule.Recommend.Intent, Preset: rule.Recommend.Preset}
	for _, version := range versions {
		r.replayVersion(&report, compiledRule{rule: rule, program: program}, recommended, version)
	}
	return report, nil
}

func (r *Rules) replayVersion(report *ReplayReport, rule compiledRule, recommended Recommendation, version ReplayVersion) {
	report.VersionsExamined++
	features := r.Features(version.Facts, version.Evidence)
	if !rule.matches(features) || !insideEnvelope(rule.rule.Envelope, features) {
		return
	}
	report.WouldFire++
	if version.Decision == nil {
		return
	}
	report.Decided++
	if recommended.Verdict(r.Observe(version.Decision)) == VerdictAgree {
		report.Agree++
	} else {
		report.Disagree++
	}
}

func sourceRates(key Key, cases []KeyedCase) ReplayReport {
	var report ReplayReport
	for _, c := range cases {
		if c.Key == key {
			report.SourceCases++
			if c.Verdict == CaseNotVerified {
				report.SourceNotVerified++
			}
		}
	}
	return report
}

type Binding struct {
	Approver   string
	Relay      string
	Approved   bool
	AuditEvery int
	RateCap    int
	ShadowExit spec.ShadowExit
	Cap        map[string]spec.Range
}

func (p Proposal) DefaultBinding(approver, relay string, approved bool) Binding {
	return Binding{Approver: approver, Relay: relay, Approved: approved, AuditEvery: defaultAuditEvery,
		RateCap: p.Rule.RateCapPerHour, ShadowExit: spec.ShadowExit{Agreeing: defaultShadowAgreeing}, Cap: p.Rule.Cap}
}

const (
	defaultAuditEvery     = 3
	defaultShadowAgreeing = 10
)

func (p Proposal) SigningBytes(binding Binding) ([]byte, error) {
	document := map[string]any{
		"domain": ruleApprovalDomain, "proposal_id": p.ProposalID, "proposal_digest": p.Digest, "tenant_id": p.TenantID,
		"rule_id": p.Rule.ID, "rule_version": p.Rule.Version, "cap": binding.Cap, "shadow_exit": binding.ShadowExit,
		"audit_every": binding.AuditEvery, "rate_cap_per_hour": binding.RateCap, "approver_id": binding.Approver,
		"relay_id": binding.Relay, "approved": binding.Approved,
	}
	signing, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode the approval of proposal %s: %w", p.ProposalID, err)
	}
	return signing, nil
}

func (p Proposal) ApprovedRule(binding Binding, approvalDigest string) spec.Rule {
	rule := p.Rule
	exit := binding.ShadowExit
	rule.ShadowExit, rule.AuditEvery, rule.RateCapPerHour, rule.Cap = &exit, binding.AuditEvery, binding.RateCap, binding.Cap
	rule.Mode = spec.RuleModeShadow
	rule.Provenance.ApprovalDigest = approvalDigest
	return rule
}
