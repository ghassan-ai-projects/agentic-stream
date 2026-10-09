package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeEventBodyDigestsPayloadBytes(t *testing.T) {
	t.Parallel()
	encoded, err := EncodeEventBody(map[string]any{"celsius": 30.5}, map[string]any{"grade": "A"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeEventBody(map[string]any{"celsius": 30.5}, map[string]any{"grade": "A"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded.PayloadJSON) != string(again.PayloadJSON) || len(encoded.PayloadSHA256) != 32 {
		t.Fatalf("encoding is not stable: %+v", encoded)
	}
	if string(encoded.PayloadSHA256) != string(contentSum(encoded.PayloadJSON)) {
		t.Fatalf("digest %x is not the SHA-256 of the stored payload bytes %s", encoded.PayloadSHA256, encoded.PayloadJSON)
	}
	var quality map[string]any
	if err := json.Unmarshal(encoded.QualityJSON, &quality); err != nil || quality["grade"] != "A" {
		t.Fatalf("quality encoding = %s err=%v", encoded.QualityJSON, err)
	}
}

func TestEncodeEventBodyNamesWhichPartCannotBeEncoded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		data    map[string]any
		quality any
		want    string
	}{
		{"payload", map[string]any{"bad": make(chan int)}, nil, "marshal payload"},
		{"quality", nil, make(chan int), "marshal quality"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := EncodeEventBody(tc.data, tc.quality); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadLimitDefaultsToThousand(t *testing.T) {
	t.Parallel()
	for limit, want := range map[int]int{-5: 1000, 0: 1000, 1: 1, 7: 7, 2000: 2000} {
		if got := ReadLimit(limit); got != want {
			t.Errorf("ReadLimit(%d) = %d, want %d", limit, got, want)
		}
	}
}

func TestDecodePayloadRoundTripsAndRefusesNonObjects(t *testing.T) {
	t.Parallel()
	data, err := DecodePayload([]byte(`{"celsius":30.5,"tags":["a"]}`))
	if err != nil || data["celsius"] != 30.5 {
		t.Fatalf("data = %v err=%v", data, err)
	}
	if _, err := DecodePayload([]byte(`[1]`)); err == nil || !strings.Contains(err.Error(), "unmarshal payload") {
		t.Fatalf("err = %v, want unmarshal payload", err)
	}
}
