package domain

import "testing"

func TestDocumentFieldsProjectWithoutCoercion(t *testing.T) {
	t.Parallel()
	document := map[string]any{"name": "pump", "count": float64(7), "text_number": "7", "number_text": float64(3)}
	if got := DocumentString(document, "name"); got != "pump" {
		t.Fatalf("DocumentString(name) = %q, want pump", got)
	}
	if got := DocumentString(document, "number_text"); got != "" {
		t.Fatalf("DocumentString on a number = %q, want empty", got)
	}
	if got := DocumentString(document, "absent"); got != "" {
		t.Fatalf("DocumentString on an absent field = %q, want empty", got)
	}
	if got := DocumentInt(document, "count"); got != 7 {
		t.Fatalf("DocumentInt(count) = %d, want 7", got)
	}
	if got := DocumentInt(document, "text_number"); got != 0 {
		t.Fatalf("DocumentInt on a string = %d, want 0", got)
	}
}
