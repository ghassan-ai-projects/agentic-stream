package wire_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

var answeredTypes = []string{"HSYNC", "NCMD"}

const hostOutageExchange = "host-loss.jsonl"

func TestEveryExchangeGetsTheHostRecordsItExpects(t *testing.T) {
	t.Parallel()
	for _, file := range fixtureFiles(t, "exchanges", "*.jsonl") {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			header, steps := exchangeSteps(t, file)
			handshake := firstHostState(t, steps)
			replies := conversationReplies(t, header, steps, handshake["authority_epoch"].(json.Number))
			assertHandshake(t, replies[0], handshake)
			if filepath.Base(file) != hostOutageExchange {
				assertAnswers(t, replies[1:], steps)
			}
		})
	}
}

func conversationReplies(t *testing.T, header map[string]any, steps []map[string]any, epoch json.Number) []map[string]any {
	t.Helper()
	hostEpoch, err := epoch.Int64()
	if err != nil {
		t.Fatalf("epoch %s: %v", epoch, err)
	}
	conversation := domain.NewConversation(domain.Admission{Environment: hostEnvironment(header),
		ContractDigest: wire.ContractDigest(), Inventory: validInventory(t)}, hostEpoch, domain.Mapping{})
	var replies []map[string]any
	for _, step := range steps {
		if step["from"] != "node" {
			continue
		}
		reaction := conversation.Receive(arrivalOf(t, lineOf(t, step["record"])))
		if reaction.Dropped != "" {
			t.Errorf("the host dropped %v: %s", step["record"].(map[string]any)["type"], reaction.Dropped)
		}
		replies = append(replies, reaction.Replies...)
	}
	return replies
}

func assertHandshake(t *testing.T, got, want map[string]any) {
	t.Helper()
	for _, member := range []string{"type", "online", "refusal", "protocol_version", "session_id", "seq", "authority_epoch", "heartbeat"} {
		if !sameValue(got[member], want[member]) {
			t.Errorf("handshake STATE %s = %v, want %v", member, got[member], want[member])
		}
	}
}

func assertAnswers(t *testing.T, replies []map[string]any, steps []map[string]any) {
	t.Helper()
	var want []map[string]any
	for _, step := range steps {
		if record, _ := step["record"].(map[string]any); step["from"] == "host" && slices.Contains(answeredTypes, record["type"].(string)) {
			want = append(want, record)
		}
	}
	if len(replies) != len(want) {
		t.Fatalf("the host answered %v, want %v", replies, want)
	}
	for i := range want {
		for member, value := range want[i] {
			if !sameValue(replies[i][member], value) {
				t.Errorf("answer %d %s = %v, want %v", i, member, replies[i][member], value)
			}
		}
	}
}

func firstHostState(t *testing.T, steps []map[string]any) map[string]any {
	t.Helper()
	for _, step := range steps {
		if record, _ := step["record"].(map[string]any); step["from"] == "host" && record["type"] == "STATE" {
			return record
		}
	}
	t.Fatal("no host STATE")
	return nil
}

func validInventory(t *testing.T) domain.Inventory {
	t.Helper()
	kind, err := wire.ParseKindFile(lineOf(t, readDocument(t, filepath.Join(contractDir, "fixtures", "valid", "KIND.json"))))
	if err != nil {
		t.Fatalf("ParseKindFile = %v", err)
	}
	inventory, err := domain.Commission([]domain.Kind{kind}, nil)
	if err != nil {
		t.Fatalf("Commission = %v", err)
	}
	return inventory
}

func hostEnvironment(header map[string]any) string {
	if environment, set := header["host_environment"].(string); set {
		return environment
	}
	return domain.EnvironmentSimulated
}

func sameValue(got, want any) bool {
	return string(lineOfValue(got)) == string(lineOfValue(want))
}

func lineOfValue(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
