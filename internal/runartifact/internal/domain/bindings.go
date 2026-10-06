package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// PolicyDocuments are the policy module's pure functions the artifact needs: the
// digest of a policy version and its canonical definition. The caller supplies
// them so this layer never imports the policy module.
type PolicyDocuments struct {
	DigestForVersion func(version string) (string, error)
	Canonical        func(version string) map[string]any
}

// BoundPolicy returns the canonical policy document for a recorded evaluation,
// requiring the recorded digest to match the version's digest.
func (p PolicyDocuments) BoundPolicy(version, digest string) ([]byte, error) {
	expected, err := p.DigestForVersion(version)
	if err != nil {
		return nil, fmt.Errorf("digest current policy input: %w", err)
	}
	if expected != digest {
		return nil, fmt.Errorf("policy evaluation digest does not match policy version %q", version)
	}
	return CanonicalFile(p.Canonical(version))
}

// VerifyBindings requires the manifest digests to match the canonical spec and
// policy documents. The spec is checked before the policy.
func (p PolicyDocuments) VerifyBindings(manifest Manifest, specData, policyData []byte) error {
	if err := verifySpecBinding(manifest.SpecDigest, specData); err != nil {
		return err
	}
	return p.verifyPolicyBinding(manifest.PolicyDigest, policyData)
}

func verifySpecBinding(digest string, data []byte) error {
	if digest == "" {
		return nil
	}
	var document any
	if err := json.Unmarshal(trimmed(data), &document); err != nil {
		return fmt.Errorf("decode canonical spec: %w", err)
	}
	expected, err := canonicaljson.Digest(canonicaljson.DomainSpec, document)
	if err != nil {
		return fmt.Errorf("digest canonical spec: %w", err)
	}
	if expected != digest {
		return fmt.Errorf("manifest spec digest %q does not match canonical spec %q", digest, expected)
	}
	return nil
}

func policyVersion(data []byte) (string, error) {
	var document map[string]any
	if err := json.Unmarshal(trimmed(data), &document); err != nil {
		return "", fmt.Errorf("decode canonical policy: %w", err)
	}
	version, _ := document["policy_version"].(string)
	return version, nil
}

func (p PolicyDocuments) verifyPolicyBinding(digest string, data []byte) error {
	if digest == "" {
		return nil
	}
	version, err := policyVersion(data)
	if err != nil {
		return err
	}
	expected, err := p.DigestForVersion(version)
	if err != nil {
		return fmt.Errorf("digest canonical policy: %w", err)
	}
	if expected != digest {
		return fmt.Errorf("manifest policy digest %q does not match canonical policy %q", digest, expected)
	}
	return nil
}
