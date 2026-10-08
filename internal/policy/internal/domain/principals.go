package domain

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

type PrincipalDocument struct {
	Tenant     string           `yaml:"tenant" json:"tenant"`
	Principals []PrincipalEntry `yaml:"principals" json:"principals"`
	Roles      []RoleEntry      `yaml:"roles" json:"roles"`
}

type PrincipalEntry struct {
	ID        string `yaml:"id" json:"id"`
	PublicKey string `yaml:"public_key,omitempty" json:"public_key,omitempty"`
	Status    string `yaml:"status,omitempty" json:"status,omitempty"`
}

type RoleEntry struct {
	ID          string           `yaml:"id" json:"id"`
	Name        string           `yaml:"name" json:"name"`
	Members     []string         `yaml:"members" json:"members"`
	Authorities []AuthorityEntry `yaml:"authorities" json:"authorities"`
}

type AuthorityEntry struct {
	Entity string   `yaml:"entity" json:"entity"`
	Risks  []string `yaml:"risks" json:"risks"`
}

type PrincipalSummary struct {
	Tenant      string `json:"tenant"`
	Active      int    `json:"active_principals"`
	Disabled    int    `json:"disabled_principals"`
	Roles       int    `json:"roles"`
	Memberships int    `json:"memberships"`
	Authorities int    `json:"authorities"`
}

var approvableRisks = []string{"R0", "R1", "R2"}

func ParsePrincipalDocument(data []byte) (PrincipalDocument, error) {
	var document PrincipalDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return PrincipalDocument{}, fmt.Errorf("decode principal document: %w", err)
	}
	return document, document.Validate()
}

func (d PrincipalDocument) Validate() error {
	if d.Tenant == "" {
		return errors.New("principal document needs a tenant")
	}
	keyed, err := d.validatePrincipals()
	if err != nil {
		return err
	}
	return d.validateRoles(keyed)
}

func (d PrincipalDocument) validatePrincipals() (map[string]bool, error) {
	keyed := make(map[string]bool, len(d.Principals))
	for _, principal := range d.Principals {
		if _, seen := keyed[principal.ID]; seen || principal.ID == "" {
			return nil, fmt.Errorf("principal id %q is empty or repeated", principal.ID)
		}
		if err := principal.validate(); err != nil {
			return nil, err
		}
		keyed[principal.ID] = principal.PublicKey != ""
	}
	return keyed, nil
}

func (p PrincipalEntry) validate() error {
	if p.Status != "" && p.Status != "active" && p.Status != "disabled" {
		return fmt.Errorf("principal %s status %q is not active or disabled", p.ID, p.Status)
	}
	if p.PublicKey == "" {
		return nil
	}
	if _, err := p.KeyBytes(); err != nil {
		return err
	}
	return nil
}

func (p PrincipalEntry) KeyBytes() ([]byte, error) {
	if p.PublicKey == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(p.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("principal %s public key is not a base64 Ed25519 key", p.ID)
	}
	return key, nil
}

func (p PrincipalEntry) EffectiveStatus() string {
	if p.Status == "" {
		return "active"
	}
	return p.Status
}

func (d PrincipalDocument) validateRoles(keyed map[string]bool) error {
	seen := map[string]bool{}
	for _, role := range d.Roles {
		if role.ID == "" || role.Name == "" || seen[role.ID] || seen["name:"+role.Name] {
			return fmt.Errorf("role %q needs a unique id and name", role.ID)
		}
		seen[role.ID], seen["name:"+role.Name] = true, true
		if err := role.validate(keyed); err != nil {
			return err
		}
	}
	return nil
}

func (r RoleEntry) validate(keyed map[string]bool) error {
	for _, member := range r.Members {
		hasKey, declared := keyed[member]
		if !declared || !hasKey {
			return fmt.Errorf("role %s member %q is not a declared principal with a public key", r.ID, member)
		}
	}
	for _, authority := range r.Authorities {
		if err := authority.validate(r.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a AuthorityEntry) validate(role string) error {
	if a.Entity == "" || len(a.Risks) == 0 {
		return fmt.Errorf("role %s authority needs an entity and at least one risk", role)
	}
	for _, risk := range a.Risks {
		if !slices.Contains(approvableRisks, risk) {
			return fmt.Errorf("role %s authority risk %q is not approvable (R0, R1 or R2)", role, risk)
		}
	}
	return nil
}
