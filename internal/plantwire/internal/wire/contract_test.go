package wire

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestTheVendoredContractMatchesItsPinnedDigest(t *testing.T) {
	t.Parallel()
	var body bytes.Buffer
	entries, err := schemaFiles.ReadDir(schemaDir)
	if err != nil {
		t.Fatalf("read schemas: %v", err)
	}
	for _, entry := range entries {
		raw, err := schemaFiles.ReadFile(schemaDir + "/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("canonical %s: %v", entry.Name(), err)
		}
		body.WriteString(entry.Name() + "\n" + string(canonical) + "\n")
	}
	if got := canonicaljson.ContentDigest(body.Bytes()); got != ContractDigest() {
		t.Fatalf("the schemas give %s, DIGEST pins %s: a contract change needs a reviewed DIGEST and a sync", got, ContractDigest())
	}
}

func TestTheVendoredContractIsTheVersionTheCodeSpeaks(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(vendoredDir, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != domain.ContractVersion {
		t.Fatalf("VERSION is %s, the code speaks %s: change both together", got, domain.ContractVersion)
	}
}

const vendoredDir = "plantwire-contract"

var safetyFloor = map[string][]string{
	"DCMD":     {"command_id", "idempotency_key", "device", "command", "args", "expected_boot_id", "deadline_ns", "not_before_ns", "authority_epoch", "policy_digest", "fallback_on_lease_loss"},
	"NBIRTH":   {"protocol_min", "protocol_max", "contract_version", "contract_digest", "environment", "bd_seq", "capability_digest", "kinds", "boards"},
	"DBIRTH":   {"device", "kind", "board", "boot_id", "birth_time_ns", "metrics"},
	"DDATA":    {"device", "boot_id", "historical", "metrics"},
	"STATE":    {"online", "heartbeat", "authority_epoch", "protocol_min", "protocol_max"},
	"DSTATE":   {"boot_id", "self_reported", "safe_state"},
	"DRECEIPT": {"command_id", "status", "boot_id"},
	"DRESULT":  {"command_id", "reported_outcome", "boot_id"},
}

var safetyValues = map[string]map[string][]string{
	"DRECEIPT": {"reason": {"expired", "wrong_boot", "duplicate", "stale_epoch", "policy_not_allowed", "interlocked", "out_of_range", "locked_out", "maintenance_isolated", "local_control", "permissive_not_met", "session_ending"}},
	"DRESULT":  {"reported_outcome": {"unknown"}},
	"DCMD":     {"fallback_on_lease_loss": {"safe_off"}},
	"NBIRTH":   {"environment": {"simulated", "hardware", "mixed"}},
	"STATE":    {"refusal": {"protocol_invalid", "environment_mismatch", "contract_unsupported", "version_unsupported", "inventory_mismatch"}},
}

func TestTheSafetyMembersOfTheVendoredSchemasCannotBeWeakened(t *testing.T) {
	t.Parallel()
	for _, record := range slices.Sorted(maps.Keys(safetyFloor)) {
		schema := embeddedSchema(t, record)
		required, _ := schema["required"].([]any)
		for _, member := range safetyFloor[record] {
			if !slices.Contains(required, any(member)) {
				t.Errorf("%s no longer requires %s", record, member)
			}
		}
		for member, values := range safetyValues[record] {
			enum := schema["properties"].(map[string]any)[member].(map[string]any)["enum"].([]any)
			for _, value := range values {
				if !slices.Contains(enum, any(value)) {
					t.Errorf("%s.%s no longer offers %s", record, member, value)
				}
			}
		}
	}
	if embeddedSchema(t, "DCMD")["additionalProperties"] != false {
		t.Error("DCMD no longer refuses unknown members")
	}
}

func TestTheVendoredContractIsByteIdenticalToItsMasterWhenOneIsConfigured(t *testing.T) {
	t.Parallel()
	master := os.Getenv("PLANT_WIRE_CONTRACT_DIR")
	if master == "" {
		t.Skip("PLANT_WIRE_CONTRACT_DIR is not set; make plant-wire-sync sets it")
	}
	if local, upstream := folderContents(t, os.DirFS(vendoredDir)), folderContents(t, os.DirFS(master)); !maps.Equal(local, upstream) {
		t.Errorf("%s differs from the master %s: run make plant-wire-sync", vendoredDir, master)
	}
}

func embeddedSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := schemaFiles.ReadFile(schemaDir + "/" + name + schemaSuffix)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return schema
}

func folderContents(t *testing.T, folder fs.FS) map[string]string {
	t.Helper()
	contents := map[string]string{}
	err := fs.WalkDir(folder, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(folder, path)
		contents[path] = string(raw)
		return err
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return contents
}
