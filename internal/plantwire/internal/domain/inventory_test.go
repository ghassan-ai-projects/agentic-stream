package domain_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestKindDigestIsTheSHA256OfTheCanonicalEntryWithoutItsDigest(t *testing.T) {
	t.Parallel()
	const want = "sha256:5faeec2369482fdce7d35ddc834aa96a1797f51c1b0a1f42f5ce40f70ad00830"
	for name, kind := range map[string]domain.Kind{"kind file": valveKind(), "announced entry": announced(t, valveKind())} {
		if got, err := domain.KindDigest(kind); err != nil || got != want {
			t.Errorf("%s: KindDigest = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestKindDigestRefusesAnEntryWithoutACanonicalForm(t *testing.T) {
	t.Parallel()
	kind := domain.Kind{"kind": "valve", "limit": json.Number("9007199254740993")}
	if _, err := domain.KindDigest(kind); !errors.Is(err, domain.ErrKindNotCanonical) {
		t.Fatalf("KindDigest = %v, want ErrKindNotCanonical", err)
	}
}

func TestCommissionRefusesAKindFileThatBreaksAKindRule(t *testing.T) {
	t.Parallel()
	emptyRange := valveKind()
	emptyRange["metrics"] = []any{map[string]any{"name": "opening", "instrument_range": map[string]any{"min": 5.0, "max": json.Number("5")}}}
	cases := []struct {
		name  string
		files []domain.Kind
		want  error
	}{
		{"a kind file that carries its digest", []domain.Kind{announced(t, valveKind())}, domain.ErrKindDigestInFile},
		{"a range whose min is not below its max", []domain.Kind{emptyRange}, domain.ErrEmptyRange},
		{"one kind in two files, names folded", []domain.Kind{valveKind(), {"kind": "VALVE"}}, domain.ErrKindCommissionedTwice},
		{"a kind with no canonical form", []domain.Kind{{"kind": "big", "limit": json.Number("1e400")}}, domain.ErrKindNotCanonical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := domain.Commission(tc.files, nil); !errors.Is(err, tc.want) {
				t.Fatalf("Commission = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestInventoryQuarantinesEveryKindNoKindFileCommissionsByDigest(t *testing.T) {
	t.Parallel()
	inventory, err := domain.Commission([]domain.Kind{valveKind()}, nil)
	if err != nil {
		t.Fatalf("commission: %v", err)
	}
	changed := valveKind()
	changed["commands"] = []any{}
	birth := birthOf(t, func(b *domain.Birth) {
		b.Kinds = []domain.Kind{announced(t, domain.Kind{"kind": "pump", "metrics": []any{}, "commands": []any{}}), announced(t, valveKind())}
	})
	if got := inventory.Quarantined(birth); !slices.Equal(got, []string{"pump"}) {
		t.Errorf("Quarantined = %v, want [pump]", got)
	}
	birth.Kinds = []domain.Kind{announced(t, changed)}
	if got := inventory.Quarantined(birth); !slices.Equal(got, []string{"valve"}) {
		t.Errorf("Quarantined of a changed valve = %v, want [valve]", got)
	}
}
