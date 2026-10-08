package kernel_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestDurableTimeTextOrdersChronologicallyAndRoundTrips(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	instants := []time.Time{base, base.Add(500 * time.Millisecond), base.Add(time.Second), base.Add(time.Nanosecond), base.In(time.FixedZone("plus2", 2*3600)).Add(time.Hour)}
	for _, a := range instants {
		parsed, err := kernel.ParseTime(kernel.FormatTime(a))
		if err != nil || !parsed.Equal(a) || parsed.Location() != time.UTC {
			t.Fatalf("round trip of %v = %v, %v", a, parsed, err)
		}
		for _, b := range instants {
			if a.Before(b) != (kernel.FormatTime(a) < kernel.FormatTime(b)) {
				t.Fatalf("text order of %v and %v disagrees with instant order", a, b)
			}
		}
	}
	if _, err := kernel.ParseTime("not a time"); err == nil {
		t.Fatal("ParseTime accepted text that is not a timestamp")
	}
	if got, want := kernel.FormatTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "2026-01-01T00:00:00.000000000Z"; got != want {
		t.Fatalf("FormatTime = %q, want %q", got, want)
	}
}

func TestDigestTextRoundTripsAndRejectsNonCanonicalForms(t *testing.T) {
	t.Parallel()
	sum := make([]byte, 32)
	sum[0], sum[31] = 0xab, 0xcd
	text := kernel.EncodeDigest(sum)
	if text != "sha256:ab000000000000000000000000000000000000000000000000000000000000cd" {
		t.Fatalf("EncodeDigest = %q", text)
	}
	if decoded, err := kernel.DecodeDigest(text); err != nil || string(decoded) != string(sum) {
		t.Fatalf("DecodeDigest = %x, %v", decoded, err)
	}
	for name, bad := range map[string]string{
		"no prefix":  "ab000000000000000000000000000000000000000000000000000000000000cd",
		"uppercase":  "sha256:AB000000000000000000000000000000000000000000000000000000000000CD",
		"short":      "sha256:abcd",
		"not hex":    "sha256:zz000000000000000000000000000000000000000000000000000000000000cd",
		"wrong algo": "md5:ab000000000000000000000000000000000000000000000000000000000000cd",
	} {
		if _, err := kernel.DecodeDigest(bad); err == nil {
			t.Fatalf("%s: DecodeDigest accepted %q", name, bad)
		}
	}
}

func TestParseStoredTimeAcceptsOnlyTheTextFormatTimeWrites(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 9, 14, 0, 0, 5, time.UTC)
	if parsed, err := kernel.ParseStoredTime(kernel.FormatTime(at)); err != nil || !parsed.Equal(at) {
		t.Fatalf("ParseStoredTime(FormatTime) = %v, %v", parsed, err)
	}
	for _, text := range []string{"", "garbage", "2026-10-09T14:00:00Z", "2026-10-09T14:00:00.5Z", "2026-10-09T16:00:00.000000000+02:00", "2026-10-09 14:00:00.000000000Z"} {
		if _, err := kernel.ParseStoredTime(text); err == nil {
			t.Errorf("ParseStoredTime(%q) accepted text FormatTime never writes", text)
		}
	}
}
