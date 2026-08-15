package canonicaljson

import (
	"encoding/json"
	"testing"
)

// P1 cross-repo parity: the Ruby frame verifies the prompt digest with the
// SAME domain + canonicalization as the Go assembler, and the Ruby
// DiagnosisCatalog.verify_wire binds the diagnosis-catalog digest over the
// PARSED catalog ARRAY (not a wrapped string). The frozen vectors below are
// byte-identical to the Ruby computations (verified both sides); a change on
// either side breaks the other side's fail-closed gate.
func TestP1CrossRepoDigestParity(t *testing.T) {
	prompt := "You are the pond supervisor's diagnostic assistant."
	promptDigest, err := Digest(DomainPrompt, map[string]any{"version": "1.0", "text": prompt})
	if err != nil {
		t.Fatalf("prompt digest: %v", err)
	}
	const wantPrompt = "sha256:d288842cb90fe718a43e494a527b95cea1fd94e2a673bc65507e3bc3d93bf45a"
	if promptDigest != wantPrompt {
		t.Fatalf("prompt digest = %s, want %s (Ruby-computed vector)", promptDigest, wantPrompt)
	}

	catalog := `[{"code":"unknown","description":"no confident diagnosis"}]`
	var parsed []any
	if err := json.Unmarshal([]byte(catalog), &parsed); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	catalogDigest, err := Digest(DomainDiagnosisCatalog, parsed)
	if err != nil {
		t.Fatalf("catalog digest: %v", err)
	}
	const wantCatalog = "sha256:50ef8a500d51c12bb1134738cb559bfb9541ef286ba43d0e0558963bf384e553"
	if catalogDigest != wantCatalog {
		t.Fatalf("catalog digest = %s, want %s (Ruby-computed vector)", catalogDigest, wantCatalog)
	}
}
