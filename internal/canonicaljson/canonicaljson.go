// Package canonicaljson produces JCS-compatible JSON for contract identity and
// digest computation, with the runtime's strict producer validation profile.
// It is a thin facade over internal/domain; see README.md.
package canonicaljson

import "github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson/internal/domain"

// Domain identifies the contract namespace included in a digest preimage. The
// newline is part of every domain and prevents concatenation ambiguity.
type Domain = domain.Domain

// Digest domains, one per contract namespace.
const (
	DomainSnapshot          = domain.DomainSnapshot
	DomainSpec              = domain.DomainSpec
	DomainDecision          = domain.DomainDecision
	DomainIntent            = domain.DomainIntent
	DomainCommand           = domain.DomainCommand
	DomainEvent             = domain.DomainEvent
	DomainOutcome           = domain.DomainOutcome
	DomainSituationState    = domain.DomainSituationState
	DomainEnvelope          = domain.DomainEnvelope
	DomainPrompt            = domain.DomainPrompt
	DomainObjective         = domain.DomainObjective
	DomainDiagnosisCatalog  = domain.DomainDiagnosisCatalog
	DomainIntentCatalog     = domain.DomainIntentCatalog
	DomainApproval          = domain.DomainApproval
	DomainPolicy            = domain.DomainPolicy
	DomainCapabilityCatalog = domain.DomainCapabilityCatalog
	DomainShadowComparison  = domain.DomainShadowComparison
)

// Marshal returns the canonical JSON encoding of v.
func Marshal(v any) ([]byte, error) { return domain.Marshal(v) }

// Digest computes a domain-separated SHA-256 digest of v as a "sha256:<hex>"
// reference.
func Digest(domainName Domain, v any) (string, error) { return domain.Digest(domainName, v) }

// DecodeDigest converts a canonical "sha256:<hex>" digest into its 32-byte
// storage representation. Unprefixed digests are invalid contract values.
func DecodeDigest(digest string) ([]byte, error) { return domain.DecodeDigest(digest) }

// Verify recomputes the digest of v in the domain and compares it with digest.
func Verify(domainName Domain, v any, digest string) bool {
	return domain.Verify(domainName, v, digest)
}

// EncodeDigest formats a raw 32-byte SHA-256 sum as a "sha256:<hex>"
// reference. It is the inverse of DecodeDigest.
func EncodeDigest(sum []byte) string { return domain.EncodeDigest(sum) }

// ContentDigest returns the "sha256:<hex>" reference of data's raw SHA-256,
// without a domain prefix. It identifies stored canonical documents.
func ContentDigest(data []byte) string { return domain.ContentDigest(data) }

// VerifyStored checks a stored canonical JSON document against its raw
// SHA-256. A non-canonical document is reported before a digest mismatch.
func VerifyStored(data, digest []byte) error { return domain.VerifyStored(data, digest) }
