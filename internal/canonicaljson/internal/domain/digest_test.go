package domain

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

const testDomain Domain = "situation-runtime/test/v1\n"

func TestDomainsAreTheFrozenDigestNamespaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		domain Domain
		want   string
	}{
		{DomainSnapshot, "situation-runtime/snapshot/v1\n"},
		{DomainSpec, "situation-runtime/spec/v1\n"},
		{DomainDecision, "situation-runtime/decision/v1\n"},
		{DomainIntent, "situation-runtime/intent/v1\n"},
		{DomainCommand, "situation-runtime/command/v1\n"},
		{DomainEvent, "situation-runtime/event/v1\n"},
		{DomainOutcome, "situation-runtime/outcome/v1\n"},
		{DomainSituationState, "situation-runtime/situation-state/v1\n"},
		{DomainEnvelope, "situation-runtime/envelope/v1\n"},
		{DomainPrompt, "situation-runtime/prompt/v1\n"},
		{DomainObjective, "situation-runtime/objective/v1\n"},
		{DomainDiagnosisCatalog, "situation-runtime/diagnosis-catalog/v1\n"},
		{DomainIntentCatalog, "situation-runtime/intent-catalog/v1\n"},
		{DomainApproval, "situation-runtime/approval-assertion/v1\n"},
		{DomainPolicy, "situation-runtime/policy/v1\n"},
		{DomainCapabilityCatalog, "situation-runtime/capability-catalog/v1\n"},
		{DomainShadowComparison, "situation-runtime/shadow-comparison/v1\n"},
	}
	shape := regexp.MustCompile(`^situation-runtime/[a-z-]+/v1\n$`)
	seen := map[Domain]bool{}
	for _, tt := range tests {
		if string(tt.domain) != tt.want || !shape.MatchString(tt.want) {
			t.Errorf("domain %q, want %q in the form situation-runtime/<name>/v1 plus a newline", tt.domain, tt.want)
		}
		if seen[tt.domain] {
			t.Errorf("domain %q names two namespaces", tt.domain)
		}
		seen[tt.domain] = true
	}
}

func TestDigestIgnoresKeyOrderAndIsBoundToItsDomain(t *testing.T) {
	t.Parallel()
	first, err := Digest(testDomain, map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Digest(testDomain, map[string]any{"a": 1, "b": 2})
	if err != nil || first != second {
		t.Fatalf("digests of the same object differ: %s vs %s (%v)", first, second, err)
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(first) {
		t.Fatalf("digest %q is not sha256:<64 lowercase hex>", first)
	}
	other, err := Digest(DomainSnapshot, map[string]any{"a": 1, "b": 2})
	if err != nil || other == first {
		t.Fatalf("the same value digests identically in two domains: %s (%v)", other, err)
	}
	changed, err := Digest(testDomain, map[string]any{"a": 1, "b": 3})
	if err != nil || changed == first {
		t.Fatalf("a changed value kept its digest: %s (%v)", changed, err)
	}
}

func TestDigestRefusesAnEmptyDomainAndUnencodableValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		domain Domain
		value  any
		want   string
	}{
		{"empty domain", "", map[string]any{}, "empty digest domain"},
		{"non-finite number", testDomain, math.NaN(), "non-finite float"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, err := Digest(tt.domain, tt.value); err == nil || got != "" || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Digest = %q, %v; want no digest and an error containing %q", got, err, tt.want)
			}
			if got, err := DigestSum(tt.domain, tt.value); err == nil || got != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DigestSum = %x, %v; want no sum and an error containing %q", got, err, tt.want)
			}
			if canonical, sum, err := Seal(tt.domain, tt.value); err == nil || canonical != nil || sum != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Seal = %s, %x, %v; want nothing and an error containing %q", canonical, sum, err, tt.want)
			}
		})
	}
}

func TestSealCanonicalizesOnceAndMatchesDigestSum(t *testing.T) {
	t.Parallel()
	document := map[string]any{"b": 1, "a": "x"}
	canonical, sum, err := Seal(testDomain, document)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := Marshal(document)
	if string(canonical) != string(wantJSON) {
		t.Fatalf("Seal json = %s, want %s", canonical, wantJSON)
	}
	digest, err := Digest(testDomain, document)
	if err != nil || EncodeDigest(sum) != digest {
		t.Fatalf("Seal sum = %s, want %s (err %v)", EncodeDigest(sum), digest, err)
	}
	if byDigestSum, err := DigestSum(testDomain, document); err != nil || string(byDigestSum) != string(sum) {
		t.Fatalf("DigestSum = %x, want %x (err %v)", byDigestSum, sum, err)
	}
}

func TestVerifyBindsDigestToValueAndDomain(t *testing.T) {
	t.Parallel()
	document := map[string]any{"ok": true}
	digest, err := Digest(testDomain, document)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		domain Domain
		value  any
		digest string
		want   bool
	}{
		{"matching", testDomain, document, digest, true},
		{"wrong domain", DomainSnapshot, document, digest, false},
		{"changed value", testDomain, map[string]any{"ok": false}, digest, false},
		{"empty domain", "", document, digest, false},
		{"unencodable value", testDomain, math.NaN(), digest, false},
		{"empty digest", testDomain, document, "", false},
		{"truncated digest", testDomain, document, digest[:len(digest)-1], false},
		{"same length different digest", testDomain, document, flipLastDigit(digest), false},
		{"upper case digest", testDomain, document, strings.ToUpper(digest), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Verify(tt.domain, tt.value, tt.digest); got != tt.want {
				t.Fatalf("Verify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifySumBindsSumToValueAndDomain(t *testing.T) {
	t.Parallel()
	document := map[string]any{"ok": true}
	sum, err := DigestSum(testDomain, document)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		domain Domain
		value  any
		sum    []byte
		want   bool
	}{
		{"matching", testDomain, document, sum, true},
		{"wrong domain", DomainSnapshot, document, sum, false},
		{"changed value", testDomain, map[string]any{"ok": false}, sum, false},
		{"wrong length", testDomain, document, sum[:len(sum)-1], false},
		{"nil sum", testDomain, document, nil, false},
		{"empty domain", "", document, sum, false},
		{"unencodable value", testDomain, math.NaN(), sum, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := VerifySum(tt.domain, tt.value, tt.sum); got != tt.want {
				t.Fatalf("VerifySum = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeDigestNamesTheModuleInItsErrors(t *testing.T) {
	t.Parallel()
	zeros := strings.Repeat("0", 64)
	if got, err := DecodeDigest("sha256:" + zeros); err != nil || len(got) != 32 {
		t.Fatalf("DecodeDigest(valid) = %x, %v", got, err)
	}
	if got, err := DecodeDigest(zeros); err == nil || !strings.HasPrefix(err.Error(), "canonicaljson: ") {
		t.Fatalf("DecodeDigest(unprefixed) = %x, %v; want a canonicaljson-prefixed error", got, err)
	}
}

func flipLastDigit(digest string) string {
	last := "0"
	if strings.HasSuffix(digest, "0") {
		last = "1"
	}
	return digest[:len(digest)-1] + last
}
