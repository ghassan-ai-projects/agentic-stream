package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestAdmissionRefusesABirthWithTheFirstRuleItBreaks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		host    string
		devices []string
		change  func(*domain.Birth)
		refusal string
		reason  string
	}{
		{"protocol range upside down before everything else", domain.EnvironmentSimulated, nil, func(b *domain.Birth) {
			b.ProtocolMin, b.ProtocolMax, b.Environment, b.ContractVersion = 2, 1, domain.EnvironmentHardware, "9.9.9"
		}, domain.RefusalProtocolInvalid, "protocol_min 2 is above protocol_max 1"},
		{"host setting before the contract", domain.EnvironmentSimulated, nil, func(b *domain.Birth) {
			b.Environment, b.Boards, b.ContractVersion = domain.EnvironmentHardware, boards(domain.EnvironmentHardware), "9.9.9"
		}, domain.RefusalEnvironmentMismatch, "the host accepts simulated nodes only"},
		{"a simulated node with a hardware board, even for a mixed host", domain.EnvironmentMixed, nil, func(b *domain.Birth) {
			b.Boards = boards(domain.EnvironmentSimulated, domain.EnvironmentHardware)
		}, domain.RefusalEnvironmentMismatch, "/boards/1/environment"},
		{"a mixed node with simulated boards only", domain.EnvironmentMixed, nil, func(b *domain.Birth) {
			b.Environment = domain.EnvironmentMixed
		}, domain.RefusalEnvironmentMismatch, "none of its 1 boards is hardware"},
		{"a mixed node with hardware boards only", domain.EnvironmentMixed, nil, func(b *domain.Birth) {
			b.Environment, b.Boards = domain.EnvironmentMixed, boards(domain.EnvironmentHardware)
		}, domain.RefusalEnvironmentMismatch, "none of its 1 boards is simulated"},
		{"another contract version", domain.EnvironmentSimulated, nil, func(b *domain.Birth) { b.ContractVersion = "1.3.0" },
			domain.RefusalContractUnsupported, "the node announces 1.3.0"},
		{"another contract digest at the same version", domain.EnvironmentSimulated, nil, func(b *domain.Birth) {
			b.ContractDigest = strings.Replace(contractDigest, "1", "2", 1)
		}, domain.RefusalContractUnsupported, "sha256:2111"},
		{"no common protocol version", domain.EnvironmentSimulated, nil, func(b *domain.Birth) { b.ProtocolMin, b.ProtocolMax = 2, 3 },
			domain.RefusalVersionUnsupported, "the node 2 to 3"},
		{"a kind digest its entry does not give", domain.EnvironmentSimulated, nil, func(b *domain.Birth) {
			b.Kinds[0]["kind_digest"] = contractDigest
		}, domain.RefusalInventoryMismatch, `kind "valve" announces a kind_digest`},
		{"a kind announced twice in another case", domain.EnvironmentSimulated, nil, func(b *domain.Birth) {
			b.Kinds = append(b.Kinds, announced(t, domain.Kind{"kind": "Valve", "metrics": []any{}, "commands": []any{}}))
		}, domain.RefusalInventoryMismatch, `kind "Valve" is announced twice`},
		{"a commissioned device missing", domain.EnvironmentSimulated, []string{"V1", "V2"}, nil,
			domain.RefusalInventoryMismatch, `commissioned device "V2"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			admission := admissionFor(t, tc.host, tc.devices)
			verdict := admission.Admit(birthOf(t, tc.change))
			if verdict.Refusal != tc.refusal || !strings.Contains(verdict.Reason, tc.reason) || verdict.Accepted() {
				t.Fatalf("verdict = %+v, want refusal %s with a reason containing %q", verdict, tc.refusal, tc.reason)
			}
		})
	}
}

func TestAdmissionAcceptsAnAgreeingBirthAtProtocolOne(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		host   string
		change func(*domain.Birth)
	}{
		{"a simulated node at a simulated host", domain.EnvironmentSimulated, nil},
		{"a hardware node at a mixed host", domain.EnvironmentMixed, func(b *domain.Birth) {
			b.Environment, b.Boards = domain.EnvironmentHardware, boards(domain.EnvironmentHardware)
		}},
		{"a mixed node at a mixed host", domain.EnvironmentMixed, func(b *domain.Birth) {
			b.Environment, b.Boards = domain.EnvironmentMixed, boards(domain.EnvironmentHardware, domain.EnvironmentSimulated)
		}},
		{"a node speaking protocols 1 to 2", domain.EnvironmentSimulated, func(b *domain.Birth) { b.ProtocolMax = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict := admissionFor(t, tc.host, []string{"V1"}).Admit(birthOf(t, tc.change))
			if !verdict.Accepted() || verdict.ProtocolVersion != 1 {
				t.Fatalf("verdict = %+v, want accepted at protocol 1", verdict)
			}
		})
	}
}

func TestCheckEnvironmentAcceptsOnlyTheThreeSettings(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{domain.EnvironmentSimulated, domain.EnvironmentHardware, domain.EnvironmentMixed} {
		if err := domain.CheckEnvironment(setting); err != nil {
			t.Errorf("CheckEnvironment(%q) = %v, want nil", setting, err)
		}
	}
	for _, setting := range []string{"", "Mixed", "real"} {
		if err := domain.CheckEnvironment(setting); !errors.Is(err, domain.ErrUnknownEnvironment) {
			t.Errorf("CheckEnvironment(%q) = %v, want ErrUnknownEnvironment", setting, err)
		}
	}
}

func admissionFor(t *testing.T, host string, devices []string) domain.Admission {
	t.Helper()
	inventory, err := domain.Commission([]domain.Kind{valveKind()}, devices)
	if err != nil {
		t.Fatalf("commission: %v", err)
	}
	return domain.Admission{Environment: host, ContractDigest: contractDigest, Inventory: inventory}
}
