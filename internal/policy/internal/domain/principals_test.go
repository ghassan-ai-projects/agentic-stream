package domain

import (
	"strings"
	"testing"
)

const validKey = "O2onvM62pC1io6jQKm8Nc2UyFXcd4kOmOsBIoYtZ2ik="

func TestPrincipalDocumentValidation(t *testing.T) {
	t.Parallel()
	valid := "tenant: default\nprincipals:\n  - id: relay\n  - id: alice\n    public_key: " + validKey + "\nroles:\n  - id: r\n    name: thermal\n    members: [alice]\n    authorities:\n      - entity: zone-01\n        risks: [R2]\n"
	cases := []struct{ name, document, want string }{
		{"valid", valid, ""},
		{"no tenant", strings.Replace(valid, "tenant: default", "tenant: \"\"", 1), "needs a tenant"},
		{"unknown field", valid + "extra: 1\n", "field extra not found"},
		{"repeated principal", strings.Replace(valid, "id: relay", "id: alice", 1), "empty or repeated"},
		{"bad key", strings.Replace(valid, validKey, "not-a-key", 1), "not a base64 Ed25519 key"},
		{"bad status", strings.Replace(valid, "  - id: relay\n", "  - id: relay\n    status: paused\n", 1), "is not active or disabled"},
		{"member without key", strings.Replace(valid, "members: [alice]", "members: [relay]", 1), "with a public key"},
		{"undeclared member", strings.Replace(valid, "members: [alice]", "members: [bob]", 1), "not a declared principal"},
		{"unapprovable risk", strings.Replace(valid, "risks: [R2]", "risks: [R3]", 1), "is not approvable"},
		{"authority without risks", strings.Replace(valid, "risks: [R2]", "risks: []", 1), "at least one risk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParsePrincipalDocument([]byte(tc.document))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
