package policy

import "testing"

func TestDefinitionFacade(t *testing.T) {
	t.Parallel()
	digest, err := DigestForVersion("v1")
	if err != nil || digest == "" {
		t.Fatal(digest, err)
	}
	if CanonicalDocumentForVersion("v1")["policy_version"] != "v1" {
		t.Fatal("definition delegation")
	}
}
