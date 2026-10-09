package policy

import "testing"

func TestTheFacadeExportsThePolicyDefinitionAndItsDigest(t *testing.T) {
	t.Parallel()
	digest, err := DigestForVersion("v1")
	if err != nil || digest == "" {
		t.Fatalf("DigestForVersion(v1) = %q, %v", digest, err)
	}
	if version := CanonicalDocumentForVersion("v1")["policy_version"]; version != "v1" {
		t.Fatalf("policy_version = %v, want v1", version)
	}
	if _, err := DigestForVersion(""); err == nil {
		t.Fatal("a policy without a version has a digest")
	}
}
