package kernel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestFormatTimeIsUTCWithANineDigitFraction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"whole second", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "2026-01-01T00:00:00.000000000Z"},
		{"one nanosecond", time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC), "2026-01-01T00:00:00.000000001Z"},
		{"half second", time.Date(2026, 10, 9, 12, 0, 0, 500_000_000, time.UTC), "2026-10-09T12:00:00.500000000Z"},
		{"other zone converts to UTC", time.Date(2026, 10, 9, 14, 0, 0, 0, time.FixedZone("plus2", 2*3600)), "2026-10-09T12:00:00.000000000Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kernel.FormatTime(tt.at); got != tt.want {
				t.Fatalf("FormatTime(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestDurableTimeTextOrdersChronologicallyAndRoundTrips(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	instants := []time.Time{
		base, base.Add(500 * time.Millisecond), base.Add(time.Second), base.Add(time.Nanosecond),
		base.In(time.FixedZone("plus2", 2*3600)).Add(time.Hour),
	}
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
}

func TestParseTimeReadsAnyRFC3339TextIntoUTC(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, text := range []string{"2026-10-09T12:00:00Z", "2026-10-09T12:00:00.000000000Z", "2026-10-09T14:00:00+02:00"} {
		got, err := kernel.ParseTime(text)
		if err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("ParseTime(%q) = %v, %v; want %v in UTC", text, got, err, want)
		}
	}
	if _, err := kernel.ParseTime("not a time"); err == nil || !strings.Contains(err.Error(), "parse time") {
		t.Fatalf("ParseTime(garbage) = %v, want a parse time error", err)
	}
}

func TestParseStoredTimeAcceptsOnlyTheTextFormatTimeWrites(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 9, 14, 0, 0, 5, time.UTC)
	got, err := kernel.ParseStoredTime(kernel.FormatTime(at))
	if err != nil || !got.Equal(at) {
		t.Fatalf("ParseStoredTime(FormatTime) = %v, %v; want %v", got, err, at)
	}
	rejected := map[string]string{
		"empty":                "",
		"garbage":              "garbage",
		"no fraction":          "2026-10-09T14:00:00Z",
		"short fraction":       "2026-10-09T14:00:00.5Z",
		"offset instead of Z":  "2026-10-09T16:00:00.000000000+02:00",
		"space instead of T":   "2026-10-09 14:00:00.000000000Z",
		"lowercase separators": "2026-10-09t14:00:00.000000000z",
	}
	for name, text := range rejected {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := kernel.ParseStoredTime(text); err == nil {
				t.Fatalf("ParseStoredTime(%q) = %v, want an error: FormatTime never writes this text", text, got)
			}
		})
	}
	if _, err := kernel.ParseStoredTime("2026-10-09T14:00:00Z"); err == nil || !strings.Contains(err.Error(), "not durable time text") {
		t.Fatalf("a valid timestamp in another layout = %v, want the not durable time text error", err)
	}
}

func TestEncodeDigestPrefixesLowercaseHex(t *testing.T) {
	t.Parallel()
	sum := make([]byte, 32)
	sum[0], sum[31] = 0xab, 0xcd
	const want = "sha256:ab000000000000000000000000000000000000000000000000000000000000cd"
	if got := kernel.EncodeDigest(sum); got != want {
		t.Fatalf("EncodeDigest = %q, want %q", got, want)
	}
	decoded, err := kernel.DecodeDigest(want)
	if err != nil || string(decoded) != string(sum) {
		t.Fatalf("DecodeDigest(%q) = %x, %v; want %x", want, decoded, err, sum)
	}
}

func TestDecodeDigestRejectsNonCanonicalForms(t *testing.T) {
	t.Parallel()
	const hex = "ab000000000000000000000000000000000000000000000000000000000000cd"
	tests := []struct {
		name, digest, want string
	}{
		{"no prefix", hex, "prefix"},
		{"wrong algorithm", "md5:" + hex, "prefix"},
		{"empty", "", "prefix"},
		{"uppercase", "sha256:" + strings.ToUpper(hex), "lowercase"},
		{"not hex", "sha256:zz" + hex[2:], "decode digest"},
		{"odd length", "sha256:" + hex[1:], "decode digest"},
		{"short", "sha256:abcd", "2 bytes, want 32"},
		{"long", "sha256:" + hex + "00", "33 bytes, want 32"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := kernel.DecodeDigest(tt.digest)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeDigest(%q) = %x, %v; want an error containing %q", tt.digest, got, err, tt.want)
			}
		})
	}
}
