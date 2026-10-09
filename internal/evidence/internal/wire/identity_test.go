package wire

import "testing"

func TestTokenIDKeepsAnExistingIdentityAndMintsOpaqueOnesOtherwise(t *testing.T) {
	t.Parallel()
	if got, err := TokenID("fixed"); err != nil || got != "fixed" {
		t.Fatalf("existing = %q, err %v", got, err)
	}
	first, err := TokenID("")
	if err != nil {
		t.Fatal(err)
	}
	second, err := TokenID("")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 22 {
		t.Fatalf("minted identities %q and %q, want distinct 22-character values", first, second)
	}
}

func TestNewRuntimeEpochIsOpaqueAndUnique(t *testing.T) {
	t.Parallel()
	first, err := NewRuntimeEpoch()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntimeEpoch()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 22 {
		t.Fatalf("epochs %q and %q, want distinct 22-character values", first, second)
	}
}
