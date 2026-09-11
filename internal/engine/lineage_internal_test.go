package engine

import "testing"

// TestLineageIDNoConcatenationCollision guards the explainability invariant:
// distinct ordered evidence sets must produce distinct lineage identities. A
// plain byte concatenation collided on boundary-shifted IDs (["ab","c"] vs
// ["a","bc"]), which let ON CONFLICT DO NOTHING attach the wrong references to
// a situation version.
func TestLineageIDNoConcatenationCollision(t *testing.T) {
	t.Parallel()
	e := &Engine{}
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
			if got, other := e.lineageID(tc.a), e.lineageID(tc.b); got == other {
				t.Fatalf("distinct evidence sets collided: %v and %v both hash to %s", tc.a, tc.b, got)
			}
		})
	}

	// Identical evidence must be stable across calls.
	same := []string{"evt-1", "evt-2"}
	if e.lineageID(same) != e.lineageID(same) {
		t.Fatal("lineageID is not stable for identical evidence")
	}
}
