package domain

import "testing"

// TestLineageIDNoConcatenationCollision guards the explainability invariant:
// distinct ordered evidence sets must produce distinct lineage identities. A
// plain byte concatenation collided on boundary-shifted IDs (["ab","c"] vs
// ["a","bc"]), which let ON CONFLICT DO NOTHING attach the wrong references to
// a situation version.
func TestLineageIDNoConcatenationCollision(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b []string
	}{
		{"boundary shift", []string{"ab", "c"}, []string{"a", "bc"}},
		{"empty vs joined", []string{"", "abc"}, []string{"abc", ""}},
		{"count differs", []string{"abc"}, []string{"a", "b", "c"}},
		{"delimiter in id", []string{"a\x00b"}, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, other := LineageID(tc.a), LineageID(tc.b); got == other {
				t.Fatalf("distinct evidence sets collided: %v and %v both hash to %s", tc.a, tc.b, got)
			}
		})
	}
	same := []string{"evt-1", "evt-2"}
	if first := LineageID(same); LineageID(same) != first {
		t.Fatal("LineageID is not stable for identical evidence")
	}
}

func TestNewLineageBindsIdentityDigestAndReferences(t *testing.T) {
	t.Parallel()
	lineage, err := NewLineage([]string{"evt-1", "evt-2"})
	if err != nil {
		t.Fatal(err)
	}
	if lineage.ID != LineageID([]string{"evt-1", "evt-2"}) || len(lineage.Digest) != 32 || string(lineage.ReferencesJSON) != `["evt-1","evt-2"]` {
		t.Fatalf("lineage = %+v", lineage)
	}
}
