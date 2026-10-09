package domain

import "testing"

func TestDocumentFieldsProjectWithoutCoercion(t *testing.T) {
	t.Parallel()
	document := map[string]any{"name": "pump", "count": float64(7), "text_number": "7", "fraction": 2.9, "null": nil}
	strings := []struct {
		key, want string
	}{{"name", "pump"}, {"count", ""}, {"absent", ""}, {"null", ""}}
	for _, tt := range strings {
		if got := DocumentString(document, tt.key); got != tt.want {
			t.Errorf("DocumentString(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
	ints := []struct {
		key  string
		want int
	}{{"count", 7}, {"fraction", 2}, {"text_number", 0}, {"absent", 0}, {"null", 0}}
	for _, tt := range ints {
		if got := DocumentInt(document, tt.key); got != tt.want {
			t.Errorf("DocumentInt(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
	if DocumentString(nil, "name") != "" || DocumentInt(nil, "count") != 0 {
		t.Error("a nil document must project empty values")
	}
}
