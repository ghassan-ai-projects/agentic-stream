package domain

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

func TestRiskClassesAreOrderedAndClosed(t *testing.T) {
	t.Parallel()
	want := []RiskClass{"R0", "R1", "R2", "R3", "R4"}
	if got := RiskClasses(); !slices.Equal(got, want) {
		t.Fatalf("classes = %v", got)
	}
	for index, class := range want {
		if !class.Valid() || class.Rank() != index+1 {
			t.Fatalf("%s: valid=%t rank=%d", class, class.Valid(), class.Rank())
		}
	}
	for _, invalid := range []RiskClass{"", "r1", " R1", "R5", "R"} {
		if invalid.Valid() || invalid.Rank() != 0 {
			t.Fatalf("%q accepted", invalid)
		}
	}
}

func TestRiskCeilingComparesByRank(t *testing.T) {
	t.Parallel()
	for _, risk := range RiskClasses() {
		for _, ceiling := range RiskClasses() {
			if got, want := risk.AtMost(ceiling), risk.Rank() <= ceiling.Rank(); got != want {
				t.Fatalf("%s at most %s = %t", risk, ceiling, got)
			}
		}
		if risk.AtMost("") || risk.AtMost("R9") {
			t.Fatalf("%s fits an invalid ceiling", risk)
		}
	}
}

func TestRouteForEveryRiskAndApprovalFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		risk                      RiskClass
		plain, flagged            Route
		consequential, approvable bool
	}{
		{"R0", RouteAutomatic, RouteApproval, false, true},
		{"R1", RouteAutomatic, RouteApproval, false, true},
		{"R2", RouteApproval, RouteApproval, true, true},
		{"R3", RouteDenied, RouteDenied, true, false},
		{"R4", RouteDenied, RouteDenied, true, false},
		{"R5", RouteDenied, RouteDenied, false, false},
		{"", RouteDenied, RouteDenied, false, false},
	}
	for _, test := range tests {
		if got := RouteFor(test.risk, false); got != test.plain {
			t.Fatalf("%q plain route = %s", test.risk, got)
		}
		if got := RouteFor(test.risk, true); got != test.flagged {
			t.Fatalf("%q flagged route = %s", test.risk, got)
		}
		if test.risk.Consequential() != test.consequential || test.risk.Approvable() != test.approvable {
			t.Fatalf("%q consequential=%t approvable=%t", test.risk, test.risk.Consequential(), test.risk.Approvable())
		}
	}
}

func TestPolicyDocumentsDeriveFromTheRiskTable(t *testing.T) {
	t.Parallel()
	wantPolicy := map[string]any{"R0": "automatic", "R1": "automatic", "R2": "approval", "R3": "denied", "R4": "denied"}
	wantHealth := map[string]any{"R2": "denied", "R3": "denied", "R4": "denied"}
	if got := RiskPolicyDocument(); !maps.Equal(got, wantPolicy) {
		t.Fatalf("risk policy = %v", got)
	}
	if got := IncompleteSourceHealthDocument(); !maps.Equal(got, wantHealth) {
		t.Fatalf("incomplete source health = %v", got)
	}
}

func TestSchemaRiskEnumMatchesTheRiskTable(t *testing.T) {
	t.Parallel()
	data, err := schemaFiles.ReadFile("schemas/v1/intent-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			RiskClass struct {
				Enum []RiskClass `json:"enum"`
			} `json:"risk_class"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties.RiskClass.Enum; !slices.Equal(got, RiskClasses()) {
		t.Fatalf("schema enum = %v", got)
	}
}
