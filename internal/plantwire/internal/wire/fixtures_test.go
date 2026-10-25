package wire_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

var fileFixtures = []string{"KIND.json", "BOARD.json"}

func TestEveryValidRecordFixtureDecodes(t *testing.T) {
	t.Parallel()
	for _, file := range fixtureFiles(t, "valid", "*.json") {
		if slices.Contains(fileFixtures, filepath.Base(file)) {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		in, err := wire.Decode(raw)
		if err != nil || in.Invalid != nil || !in.Sequenced || in.Type+".json" != filepath.Base(file) {
			t.Errorf("%s: Decode = %+v, %v; want a valid sequenced record of its type", filepath.Base(file), in.Envelope, errors.Join(err, in.Invalid))
		}
	}
}

func TestMustIgnoreAndInjectionFixturesDecodeAsData(t *testing.T) {
	t.Parallel()
	files := append(fixtureFiles(t, "must-ignore", "*.json"), fixtureFiles(t, "injection", "*.json")...)
	for _, file := range files {
		record := readDocument(t, file)["record"]
		if in, err := wire.Decode(lineOf(t, record)); err != nil || in.Invalid != nil {
			t.Errorf("%s: an unknown member or a hostile string must still decode: %v", filepath.Base(file), errors.Join(err, in.Invalid))
		}
	}
}

func TestEveryInvalidFixtureIsRefusedAtTheMemberItNames(t *testing.T) {
	t.Parallel()
	for _, file := range fixtureFiles(t, "invalid", "*.json") {
		fixture := readDocument(t, file)
		schema, failsAt := fixture["schema"].(string), fixture["fails_at"].(string)
		if schema == "BOARD" {
			continue
		}
		var refused *wire.SchemaError
		if err := refusalOf(t, schema, fixture["record"]); !errors.As(err, &refused) || !atMember(refused.Members, failsAt) {
			t.Errorf("%s: refusal %v, want one at %s", filepath.Base(file), err, failsAt)
		}
	}
}

func TestTheValidKindFileGivesTheDigestTheValidBirthAnnounces(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(contractDir, "fixtures", "valid", "KIND.json"))
	if err != nil {
		t.Fatalf("read KIND.json: %v", err)
	}
	kind, err := wire.ParseKindFile(raw)
	if err != nil {
		t.Fatalf("ParseKindFile = %v", err)
	}
	birth := validBirth(t)
	inventory, err := domain.Commission([]domain.Kind{kind}, []string{"P1", "P2"})
	if err != nil {
		t.Fatalf("Commission = %v", err)
	}
	if quarantined := inventory.Quarantined(birth); len(quarantined) != 0 || birth.Kinds[0].Name() != kind.Name() {
		t.Fatalf("the kind file %s commissions %v, quarantined %v", kind.Name(), birth.Kinds[0].Name(), quarantined)
	}
}

func refusalOf(t *testing.T, schema string, record any) error {
	t.Helper()
	if schema == "KIND" {
		_, err := wire.ParseKindFile(lineOf(t, record))
		return err
	}
	in, err := wire.Decode(lineOf(t, record))
	if err != nil {
		return err
	}
	return in.Invalid
}

func atMember(members []string, failsAt string) bool {
	return slices.ContainsFunc(members, func(member string) bool { return strings.HasPrefix(member, failsAt) })
}

func validBirth(t *testing.T) domain.Birth {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractDir, "fixtures", "valid", "NBIRTH.json"))
	if err != nil {
		t.Fatalf("read NBIRTH.json: %v", err)
	}
	in, err := wire.Decode(raw)
	arrival := in.Arrival()
	if err != nil || arrival.Invalid != nil {
		t.Fatalf("Decode = %v", errors.Join(err, arrival.Invalid))
	}
	return *arrival.Birth
}
