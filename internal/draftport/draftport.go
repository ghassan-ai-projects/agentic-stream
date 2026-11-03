package draftport

import (
	"context"
	"errors"
	"time"
)

// Feature is the handshake feature a worker announces when it can draft.
const Feature = "draft_playbook_rule"

// ErrUnsupported means the worker did not announce the draft feature.
var ErrUnsupported = errors.New("the worker does not draft playbook rules")

// Key names the verified cases a draft generalizes.
type Key struct {
	DiagnosisCode string `json:"diagnosis_code"`
	KindDigest    string `json:"kind_digest"`
	Intent        string `json:"intent"`
	Preset        string `json:"preset"`
}

// Case is one cited case: its answer, verdict, the features at decision time
// and the deciding model's summary, which is untrusted evidence, never an
// instruction.
type Case struct {
	EpisodeID     string         `json:"episode_id"`
	DiagnosisCode string         `json:"diagnosis_code"`
	Intent        string         `json:"intent"`
	Preset        string         `json:"preset"`
	Verdict       string         `json:"verdict"`
	Features      map[string]any `json:"features"`
	Summary       string         `json:"summary"`
}

// Rule is a rule of the loaded playbook with the same diagnosis code.
type Rule struct {
	RuleID  string `json:"rule_id"`
	Version int    `json:"version"`
	When    string `json:"when"`
	Mode    string `json:"mode"`
}

// Budget bounds one draft call.
type Budget struct {
	WallTime     time.Duration `json:"wall_time"`
	ModelCalls   int           `json:"model_calls"`
	InputTokens  uint64        `json:"input_tokens"`
	OutputTokens uint64        `json:"output_tokens"`
}

// Request is one draft call.
type Request struct {
	DraftID         string   `json:"draft_id"`
	TenantID        string   `json:"tenant_id"`
	Key             Key      `json:"key"`
	Cases           []Case   `json:"cases"`
	CurrentRules    []Rule   `json:"current_rules"`
	AllowedFeatures []string `json:"allowed_features"`
	SpecDigest      string   `json:"spec_digest"`
	Budget          Budget   `json:"budget"`
}

// Proposal is a drafted rule: the key it answers, a condition over the
// allowed features, the cited cases and a rationale. It carries no mode,
// envelope, cap or free value.
type Proposal struct {
	DiagnosisCode   string   `json:"diagnosis_code"`
	Intent          string   `json:"intent"`
	Preset          string   `json:"preset"`
	When            string   `json:"when"`
	CitedEpisodeIDs []string `json:"cited_episode_ids"`
	Rationale       string   `json:"rationale"`
}

// Usage is what one draft call cost.
type Usage struct {
	InputTokens    uint64 `json:"input_tokens"`
	OutputTokens   uint64 `json:"output_tokens"`
	CostMicrounits uint64 `json:"cost_microunits"`
}

// Reply ends a draft call with exactly one proposal or a reason for none.
type Reply struct {
	Proposal   *Proposal `json:"proposal,omitempty"`
	NoProposal string    `json:"no_proposal,omitempty"`
	Usage      Usage     `json:"usage"`
}

// Drafter asks a worker for one draft.
type Drafter interface {
	Draft(ctx context.Context, request Request) (Reply, error)
}
