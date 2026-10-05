package domain

import (
	"strings"
	"testing"
)

const (
	digestLower = "sha256:abababababababababababababababababababababababababababababababab"
	digestUpper = "sha256:ABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABAB"
	digestOther = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

func TestDecideBinding(t *testing.T) {
	t.Parallel()
	bound := CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootOne, Owner: ownerA, CommandDigest: digestLower}
	with := func(change func(*CommandBinding)) CommandBinding {
		changed := bound
		change(&changed)
		return changed
	}
	tests := []struct {
		name      string
		existing  *CommandBinding
		requested CommandBinding
		wantBound bool
		wantErr   bool
	}{
		{"first binding", nil, bound, false, false},
		{"identical repeat is idempotent", &bound, bound, true, false},
		{"digest compares by value", &bound, with(func(b *CommandBinding) { b.CommandDigest = digestUpper }), true, false},
		{"different digest conflicts", &bound, with(func(b *CommandBinding) { b.CommandDigest = digestOther }), false, true},
		{"missing digest conflicts", &bound, with(func(b *CommandBinding) { b.CommandDigest = "" }), false, true},
		{"invalid digest conflicts", &bound, with(func(b *CommandBinding) { b.CommandDigest = "bad" }), false, true},
		{"different boot conflicts", &bound, with(func(b *CommandBinding) { b.Device.BootID = "boot-2" }), false, true},
		{"different owner conflicts", &bound, with(func(b *CommandBinding) { b.Owner = ownerB }), false, true},
		{"different target conflicts", &bound, with(func(b *CommandBinding) { b.Target = "led-01" }), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecideBinding(tt.existing, tt.requested)
			if got != tt.wantBound || (err != nil) != tt.wantErr {
				t.Fatalf("DecideBinding = %v, %v", got, err)
			}
		})
	}
	if !bound.Complete() || with(func(b *CommandBinding) { b.CommandID = "" }).Complete() {
		t.Fatal("binding completeness")
	}
}

func TestCheckCommandEvidence(t *testing.T) {
	t.Parallel()
	binding := &CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootOne, Owner: ownerA}
	valid := validEvidence(t, bootOne, "fan-01")
	tests := []struct {
		name    string
		binding *CommandBinding
		target  string
		mutate  func(map[string]any)
		want    string
	}{
		{"unbound command needs no device evidence", nil, "fan-01", func(map[string]any) {}, ""},
		{"valid evidence", binding, "fan-01", func(map[string]any) {}, ""},
		{"command target differs from binding", binding, "led-01", func(map[string]any) {}, "binding target does not match"},
		{"evidence target differs from binding", binding, "fan-01", func(e map[string]any) { e["target"] = "led-01" }, "evidence target does not match"},
		{"evidence for another boot", binding, "fan-01", func(e map[string]any) { e["boot_id"] = "boot-2" }, "validate device reconciliation evidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evidence := cloneEvidence(valid)
			tt.mutate(evidence)
			err := CheckCommandEvidence(tt.binding, CommandEvidence{CommandID: "cmd-1", Target: tt.target, Evidence: evidence})
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("CheckCommandEvidence = %v, want %q", err, tt.want)
			}
		})
	}
}
