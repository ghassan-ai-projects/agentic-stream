package domain

import "testing"

func TestSocketPathRuleAcceptsOnlyCleanAbsoluteUnixPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		ok   bool
	}{
		{"clean absolute path", "/tmp/evidence.sock", true},
		{"empty", "", false},
		{"relative", "relative.sock", false},
		{"URL", "unix:///tmp/x.sock", false},
		{"parent traversal", "/tmp/../x.sock", false},
		{"redundant separator", "/tmp//x.sock", false},
		{"trailing separator", "/tmp/x.sock/", false},
		{"NUL byte", "/tmp/x\x00.sock", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEvidenceSocketPath(tt.path)
			if tt.ok && err != nil || !tt.ok && (err == nil || err.Error() != "evidence socket must be a clean absolute Unix path") {
				t.Fatalf("ValidateEvidenceSocketPath(%q) = %v, want ok=%v", tt.path, err, tt.ok)
			}
		})
	}
}

func TestProtocolIdentityIsPinnedToTheWireContract(t *testing.T) {
	t.Parallel()
	if ProtocolVersion != "1.0" || ContractVersion != "1.0" || EvidenceToolsFeature != "evidence_tools.v1" {
		t.Fatalf("protocol %q contract %q feature %q, want the frozen 1.0 / 1.0 / evidence_tools.v1", ProtocolVersion, ContractVersion, EvidenceToolsFeature)
	}
	if DefaultMaxEvents != 4096 || DefaultMaxStreamBytes != 16<<20 {
		t.Fatalf("stream limits = %d events, %d bytes", DefaultMaxEvents, DefaultMaxStreamBytes)
	}
}
