package domain

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const (
	RuleModeShadow = "shadow"
	RuleModeActive = "active"

	defaultShadowExitAgreeing = 10
	defaultAuditEvery         = 3

	playbookAPIVersion = "agentic-stream/v1"
	playbookKind       = "Playbook"
)

type CompiledPlaybook struct {
	SchemaVersion string           `json:"schemaVersion"`
	Metadata      PlaybookMetadata `json:"metadata"`
	Rules         []Rule           `json:"rules"`

	CanonicalJSON []byte `json:"-"`
	Digest        string `json:"-"`
}

func (p *CompiledPlaybook) PutRule(rule Rule) {
	p.Rules = slices.DeleteFunc(p.Rules, func(kept Rule) bool { return kept.ID == rule.ID })
	p.Rules = append(p.Rules, rule)
}

type PlaybookMetadata struct {
	Name string `json:"name" yaml:"name"`
	Spec string `json:"spec" yaml:"spec"`
}

type Rule struct {
	ID             string           `json:"id" yaml:"id"`
	Version        int              `json:"version" yaml:"version"`
	DiagnosisCode  string           `json:"diagnosisCode" yaml:"diagnosisCode"`
	When           string           `json:"when" yaml:"when"`
	Recommend      Recommendation   `json:"recommend" yaml:"recommend"`
	Envelope       map[string]Range `json:"envelope" yaml:"envelope"`
	Cap            map[string]Range `json:"cap" yaml:"cap"`
	PinnedKind     PinnedKind       `json:"pinnedKind" yaml:"pinnedKind"`
	Mode           string           `json:"mode" yaml:"mode"`
	ShadowExit     *ShadowExit      `json:"shadowExit" yaml:"shadowExit,omitempty"`
	AuditEvery     int              `json:"auditEvery" yaml:"auditEvery,omitempty"`
	RateCapPerHour int              `json:"rateCapPerHour" yaml:"rateCapPerHour"`
	Provenance     Provenance       `json:"provenance" yaml:"provenance"`
	Reads          []string         `json:"reads" yaml:"-"`
}

type Recommendation struct {
	Intent string `json:"intent" yaml:"intent"`
	Preset string `json:"preset" yaml:"preset"`
}

type Range struct {
	Min float64 `json:"min" yaml:"min"`
	Max float64 `json:"max" yaml:"max"`
}

func (r Range) Within(outer Range) bool {
	return r.Min >= outer.Min && r.Max <= outer.Max
}

type PinnedKind struct {
	Name   string `json:"name" yaml:"name"`
	Digest string `json:"digest" yaml:"digest"`
}

type ShadowExit struct {
	Agreeing    int `json:"agreeing" yaml:"agreeing"`
	Disagreeing int `json:"disagreeing" yaml:"disagreeing"`
}

type Provenance struct {
	SourceEpisodes []string `json:"sourceEpisodes" yaml:"sourceEpisodes"`
	ApprovalDigest string   `json:"approvalDigest,omitempty" yaml:"approvalDigest,omitempty"`
}

type rawPlaybook struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   PlaybookMetadata `yaml:"metadata" json:"metadata"`
	Rules      []Rule           `yaml:"rules" json:"rules"`
}

func CompilePlaybook(spec *CompiledSpec, data []byte) (*CompiledPlaybook, error) {
	raw, err := parseValidatedPlaybook(data)
	if err != nil {
		return nil, err
	}
	playbook := normalizePlaybook(raw)
	if err := checkPlaybook(spec, playbook); err != nil {
		return nil, err
	}
	return sealPlaybook(playbook)
}

func parseValidatedPlaybook(data []byte) (*rawPlaybook, error) {
	if err := checkDocumentKeys(data); err != nil {
		return nil, err
	}
	var raw rawPlaybook
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode playbook yaml: %w", err)
	}
	if err := validatePlaybookSchema(&raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func normalizePlaybook(raw *rawPlaybook) *CompiledPlaybook {
	playbook := &CompiledPlaybook{SchemaVersion: raw.APIVersion, Metadata: raw.Metadata, Rules: raw.Rules}
	for i := range playbook.Rules {
		defaultRule(&playbook.Rules[i])
	}
	return playbook
}

func defaultRule(rule *Rule) {
	if rule.ShadowExit == nil {
		rule.ShadowExit = &ShadowExit{Agreeing: defaultShadowExitAgreeing}
	}
	if rule.AuditEvery == 0 {
		rule.AuditEvery = defaultAuditEvery
	}
	if rule.Provenance.SourceEpisodes == nil {
		rule.Provenance.SourceEpisodes = []string{}
	}
}

func sealPlaybook(playbook *CompiledPlaybook) (*CompiledPlaybook, error) {
	canonicalJSON, sum, err := canonicaljson.Seal(canonicaljson.DomainPlaybook, playbook)
	if err != nil {
		return nil, fmt.Errorf("seal playbook: %w", err)
	}
	playbook.CanonicalJSON = canonicalJSON
	playbook.Digest = canonicaljson.EncodeDigest(sum)
	return playbook, nil
}

func MarshalPlaybook(playbook *CompiledPlaybook) ([]byte, error) {
	document, err := yaml.Marshal(rawPlaybook{APIVersion: playbookAPIVersion, Kind: playbookKind, Metadata: playbook.Metadata, Rules: playbook.Rules})
	if err != nil {
		return nil, fmt.Errorf("encode playbook yaml: %w", err)
	}
	return document, nil
}
