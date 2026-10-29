package domain

import "testing"

func TestACommandReadsBackOnlyThroughAnArgumentNamedLikeAnOutput(t *testing.T) {
	t.Parallel()
	view := DeviceView{Outputs: []string{"level_percent", "state"}}
	tests := []struct {
		name string
		args map[string]any
		want bool
	}{
		{"an output is commanded", map[string]any{"level_percent": 45}, true},
		{"an output among other arguments", map[string]any{"work_order": "bearing_wear", "state": "alert"}, true},
		{"no output is commanded", map[string]any{"work_order": "bearing_wear"}, false},
		{"no argument", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := view.ReadsBack(tt.args); got != tt.want {
				t.Fatalf("ReadsBack(%v) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
	if (DeviceView{}).ReadsBack(map[string]any{"level_percent": 45}) {
		t.Fatal("a device with no output reads back")
	}
}
