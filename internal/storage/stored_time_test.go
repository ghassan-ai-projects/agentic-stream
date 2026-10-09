package storage_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestStoredTimeFunctionAcceptsOnlyDurableTimeText(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	whole := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		name, text string
		want       bool
	}{
		{"whole second", kernel.FormatTime(whole), true},
		{"fraction", kernel.FormatTime(whole.Add(123456789)), true},
		{"trimmed fraction", "2026-10-09T14:00:00Z", false},
		{"short fraction", "2026-10-09T14:00:00.5Z", false},
		{"offset", "2026-10-09T14:00:00.000000000+02:00", false},
		{"space separator", "2026-10-09 14:00:00.000000000Z", false},
		{"month out of range", "2026-13-09T14:00:00.000000000Z", false},
		{"hour out of range", "2026-10-09T25:00:00.000000000Z", false},
		{"digits only", "9999-99-99T99:99:99.999999999Z", false},
		{"empty", "", false},
		{"garbage", "garbage", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got bool
			if err := db.QueryRowContext(t.Context(), "SELECT stored_time_ok(?)", tc.text).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("stored_time_ok(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestStoredTimeFunctionRefusesNullAndBlobs(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	var got bool
	if err := db.QueryRowContext(t.Context(), "SELECT stored_time_ok(NULL)").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("NULL counted as readable time text")
	}
	if err := db.QueryRowContext(t.Context(), "SELECT stored_time_ok(X'323032362D31302D30395431343A30303A30302E3030303030303030305A')").Scan(&got); err != nil || got {
		t.Fatalf("a BLOB counted as readable time text: %v %v", got, err)
	}
}
