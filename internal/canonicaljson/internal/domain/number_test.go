package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func TestMarshalFormatsDoublesAsRFC8785AppendixB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		bits uint64
		want string
	}{
		{"zero", 0x0000000000000000, "0"},
		{"minimum denormal", 0x0000000000000001, "5e-324"},
		{"negative minimum denormal", 0x8000000000000001, "-5e-324"},
		{"maximum double", 0x7fefffffffffffff, "1.7976931348623157e+308"},
		{"negative maximum double", 0xffefffffffffffff, "-1.7976931348623157e+308"},
		{"2^53", 0x4340000000000000, "9007199254740992"},
		{"negative 2^53", 0xc340000000000000, "-9007199254740992"},
		{"large integral value without exponent", 0x4430000000000000, "295147905179352830000"},
		{"below 1e23", 0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{"1e23", 0x44b52d02c7e14af6, "1e+23"},
		{"above 1e23", 0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{"just below 1e21", 0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{"1e21", 0x444b1ae4d6e2ef50, "1e+21"},
		{"just below 1e-6", 0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{"1e-6", 0x3eb0c6f7a0b5ed8d, "0.000001"},
		{"shortest round trip 1", 0x41b3de4355555553, "333333333.3333332"},
		{"shortest round trip 2", 0x41b3de4355555554, "333333333.33333325"},
		{"shortest round trip 3", 0x41b3de4355555555, "333333333.3333333"},
		{"shortest round trip 4", 0x41b3de4355555556, "333333333.3333334"},
		{"shortest round trip 5", 0x41b3de4355555557, "333333333.33333343"},
		{"small negative fraction", 0xbecbf647612f3696, "-0.0000033333333333333333"},
		{"fraction beyond 2^50", 0x43143ff3c1cb0959, "1424953923781206.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, math.Float64frombits(tt.bits), tt.want)
		})
	}
}

func TestMarshalPlacesTheExponentByMagnitude(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value float64
		want  string
	}{
		{1, "1"},
		{-2, "-2"},
		{1.5, "1.5"},
		{0.002, "0.002"},
		{1e-6, "0.000001"},
		{1.5e-6, "0.0000015"},
		{1e-7, "1e-7"},
		{-1e-7, "-1e-7"},
		{1.5e-7, "1.5e-7"},
		{1e20, "100000000000000000000"},
		{1.5e20, "150000000000000000000"},
		{1e21, "1e+21"},
		{-1.5e21, "-1.5e+21"},
		{1e30, "1e+30"},
		{123456.789, "123456.789"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, tt.value, tt.want)
			requireMarshals(t, float32(tt.value), mustMarshal(t, float64(float32(tt.value))))
		})
	}
}

func TestMarshalRefusesNonFiniteAndNegativeZeroDoubles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"NaN", math.NaN(), "non-finite float"},
		{"positive infinity", math.Inf(1), "non-finite float"},
		{"negative infinity", math.Inf(-1), "non-finite float"},
		{"negative zero", math.Copysign(0, -1), "negative zero"},
		{"float32 NaN", float32(math.NaN()), "non-finite float"},
		{"float32 negative zero", float32(math.Copysign(0, -1)), "negative zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, tt.value, tt.want)
		})
	}
}

func TestMarshalAcceptsGoIntegersInsideTheExactRange(t *testing.T) {
	t.Parallel()
	const maxSafe = 1<<53 - 1
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"int", int(-42), "-42"},
		{"int8", int8(-128), "-128"},
		{"int16", int16(32767), "32767"},
		{"int32", int32(-2147483648), "-2147483648"},
		{"int64 at the safe maximum", int64(maxSafe), "9007199254740991"},
		{"int64 at the safe minimum", int64(-maxSafe), "-9007199254740991"},
		{"uint", uint(7), "7"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64 at the safe maximum", uint64(maxSafe), "9007199254740991"},
		{"zero", 0, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, tt.value, tt.want)
		})
	}
}

func TestMarshalRefusesGoIntegersBeyondTheExactRange(t *testing.T) {
	t.Parallel()
	const beyond = 1 << 53
	tests := []struct {
		name  string
		value any
	}{
		{"int64 just above", int64(beyond)},
		{"int64 just below", int64(-beyond)},
		{"int64 maximum", int64(math.MaxInt64)},
		{"int64 minimum", int64(math.MinInt64)},
		{"uint64 just above", uint64(beyond)},
		{"uint64 maximum", uint64(math.MaxUint64)},
		{"int", int(beyond)},
		{"uint", uint(beyond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, tt.value, "exceeds exact JSON number range")
		})
	}
}

func TestMarshalNormalizesRawNumberSpellings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw, want string
	}{
		{"0", "0"},
		{"0.0", "0"},
		{"1", "1"},
		{"1.0", "1"},
		{"1.50", "1.5"},
		{"1e2", "100"},
		{"1E2", "100"},
		{"1e+2", "100"},
		{"0.1e1", "1"},
		{"12e-1", "1.2"},
		{"100e-2", "1"},
		{"0.5e1", "5"},
		{"-17.2", "-17.2"},
		{"1e-7", "1e-7"},
		{"4.9e-324", "5e-324"},
		{"9007199254740991", "9007199254740991"},
		{"-9007199254740991", "-9007199254740991"},
		{"9007199254740991.0", "9007199254740991"},
		{"90071992547409910e-1", "9007199254740991"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, json.RawMessage(tt.raw), tt.want)
		})
	}
}

func TestMarshalRefusesRawNumbersThatAreNotExactlyRepresentable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw, want string
	}{
		{"9007199254740992", "unsafe JSON integer"},
		{"-9007199254740992", "unsafe JSON integer"},
		{"9007199254740993", "unsafe JSON integer"},
		{"9007199254740993.0", "unsafe JSON integer"},
		{"9007199254740993e0", "unsafe JSON integer"},
		{"90071992547409930e-1", "unsafe JSON integer"},
		{"1e21", "unsafe JSON integer"},
		{"1e400", "invalid JSON number"},
		{"-1e400", "invalid JSON number"},
		{"-0", "negative zero"},
		{"-0.0", "negative zero"},
		{"-0e0", "negative zero"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, json.RawMessage(tt.raw), tt.want)
		})
	}
}

func TestMarshalRefusesMalformedNumberText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"empty json.Number", json.Number(""), "empty JSON number"},
		{"word json.Number", json.Number("abc"), "invalid JSON number"},
		{"NaN json.Number", json.Number("NaN"), "invalid JSON number"},
		{"infinity json.Number", json.Number("Inf"), "invalid JSON number"},
		{"raw leading zero", json.RawMessage("01"), ""},
		{"raw leading plus", json.RawMessage("+1"), ""},
		{"raw bare fraction", json.RawMessage(".5"), ""},
		{"raw trailing dot", json.RawMessage("1."), ""},
		{"raw hexadecimal", json.RawMessage("0x10"), ""},
		{"raw NaN", json.RawMessage("NaN"), ""},
		{"raw Infinity", json.RawMessage("Infinity"), ""},
		{"raw dangling exponent", json.RawMessage("1e"), ""},
		{"raw lone minus", json.RawMessage("-"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, tt.value, tt.want)
		})
	}
}

func TestScaleDigitsShiftsTheDecimalPointAndReportsWhetherTheValueIsAnInteger(t *testing.T) {
	t.Parallel()
	tests := []struct {
		digits      string
		places      int
		want        string
		wantInteger bool
	}{
		{digits: "12", places: -2, want: "1200", wantInteger: true},
		{digits: "12", places: 0, want: "12", wantInteger: true},
		{digits: "1200", places: 2, want: "12", wantInteger: true},
		{digits: "1250", places: 2, want: "12", wantInteger: false},
		{digits: "000", places: 5, want: "0", wantInteger: true},
		{digits: "001", places: 5, want: "0", wantInteger: false},
	}
	for _, tt := range tests {
		got, ok := scaleDigits(tt.digits, tt.places)
		if got != tt.want || ok != tt.wantInteger {
			t.Errorf("scaleDigits(%q, %d) = %q, %v; want %q, %v", tt.digits, tt.places, got, ok, tt.want, tt.wantInteger)
		}
	}
}
