package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestEpisodeKindAcceptsOnlyTheDeclaredSpellings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value   string
		want    runtimev1.EpisodeKind
		wantErr bool
	}{
		{value: "standard", want: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE},
		{value: "reconsider", want: runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER},
		{value: "diagnose", wantErr: true},
		{value: "reconsideration", wantErr: true},
		{value: "STANDARD", wantErr: true},
		{value: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			got, err := episodeKind(tt.value)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("episodeKind(%q) = %v, %v; want %v, error=%v", tt.value, got, err, tt.want, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "unsupported episode kind") {
				t.Fatalf("episodeKind(%q) error = %v", tt.value, err)
			}
		})
	}
}

func TestEpisodeLaneAcceptsOnlyDeclaredLanes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value   string
		want    runtimev1.EpisodeLane
		wantErr bool
	}{
		{value: spec.LaneFast, want: runtimev1.EpisodeLane_EPISODE_LANE_FAST},
		{value: spec.LaneDeep, want: runtimev1.EpisodeLane_EPISODE_LANE_DEEP},
		{value: "batch", wantErr: true},
		{value: "FAST", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			got, err := episodeLane(tt.value)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("episodeLane(%q) = %v, %v; want %v, error=%v", tt.value, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestRiskCeilingMapsEveryClassAndNothingElse(t *testing.T) {
	t.Parallel()
	want := []runtimev1.RiskClass{
		runtimev1.RiskClass_RISK_CLASS_R0, runtimev1.RiskClass_RISK_CLASS_R1, runtimev1.RiskClass_RISK_CLASS_R2,
		runtimev1.RiskClass_RISK_CLASS_R3, runtimev1.RiskClass_RISK_CLASS_R4,
	}
	for index, class := range contractstest.RiskClasses() {
		got, err := riskClass(class)
		if err != nil || got != want[index] {
			t.Errorf("riskClass(%s) = %v, %v; want %v", class, got, err, want[index])
		}
	}
	for _, value := range []string{"", "r1", " R1 ", "R5"} {
		if got, err := riskClass(value); err == nil {
			t.Errorf("riskClass(%q) = %v, accepted", value, got)
		}
	}
}

func TestDispatchPolicyTreatsAnythingButActiveAsShadow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		policy string
		want   runtimev1.DispatchPolicy
	}{
		{policy: spec.DispatchActive, want: runtimev1.DispatchPolicy_DISPATCH_POLICY_ACTIVE},
		{policy: spec.DispatchShadow, want: runtimev1.DispatchPolicy_DISPATCH_POLICY_SHADOW},
		{policy: "", want: runtimev1.DispatchPolicy_DISPATCH_POLICY_SHADOW},
		{policy: "bogus", want: runtimev1.DispatchPolicy_DISPATCH_POLICY_SHADOW},
	}
	for _, tt := range tests {
		t.Run("policy "+tt.policy, func(t *testing.T) {
			t.Parallel()
			if got := dispatchPolicyEnum(tt.policy); got != tt.want {
				t.Fatalf("dispatchPolicyEnum(%q) = %v, want %v", tt.policy, got, tt.want)
			}
		})
	}
}
