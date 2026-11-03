package campaign

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestCarefulWaitsThenApprovesOnlyACleanCoveredRule(t *testing.T) {
	t.Parallel()
	careful := Careful{Coverage: 3, After: 4 * time.Hour}
	for name, c := range map[string]struct {
		pending         Pending
		approved, ready bool
	}{
		"too early":    {Pending{Rule: true, Replay: ReplayReport{Agree: 5}, Waited: time.Hour}, false, false},
		"clean":        {Pending{Rule: true, Replay: ReplayReport{Agree: 3}, Waited: 5 * time.Hour}, true, true},
		"disagreement": {Pending{Rule: true, Replay: ReplayReport{Agree: 5, Disagree: 1}, Waited: 5 * time.Hour}, false, true},
		"thin":         {Pending{Rule: true, Replay: ReplayReport{Agree: 2}, Waited: 5 * time.Hour}, false, true},
		"action":       {Pending{Waited: 5 * time.Hour}, true, true},
	} {
		if approved, ready := careful.Decide(c.pending); approved != c.approved || ready != c.ready {
			t.Fatalf("%s: careful = %v/%v, want %v/%v", name, approved, ready, c.approved, c.ready)
		}
	}
	if approved, ready := (ApproveAll{}).Decide(Pending{}); !approved || !ready {
		t.Fatal("approve-all did not approve")
	}
}

type fakeEndpoint struct {
	key      ed25519.PublicKey
	mu       sync.Mutex
	resolved map[string]bool
}

func (f *fakeEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
	signing := []byte(id + r.URL.Query().Get("approved"))
	if r.Header.Get("Authorization") != "Bearer token" || id == "missing" {
		http.Error(w, "refused", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		f.present(w, id, signing)
		return
	}
	f.resolve(w, r, id)
}

func (f *fakeEndpoint) present(w http.ResponseWriter, id string, signing []byte) {
	presented := map[string]any{"signing_bytes": signing}
	if strings.HasPrefix(id, "rprop_") {
		presented["proposal"] = map[string]any{"replay": map[string]any{"agree": 4}}
	}
	_ = json.NewEncoder(w).Encode(presented)
}

func (f *fakeEndpoint) resolve(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Approved  bool   `json:"approved"`
		Signature []byte `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !ed25519.Verify(f.key, []byte(id+map[bool]string{true: "true", false: "false"}[body.Approved]), body.Signature) {
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}
	f.mu.Lock()
	f.resolved[id] = body.Approved
	f.mu.Unlock()
	_, _ = w.Write([]byte(`{}`))
}

func pendingDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	_, err := db.ExecContext(t.Context(), `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, approval_json)
		VALUES ('appr_1', 'int_1', 'pending', '2026-01-01T00:00:00.000000000Z', '2026-01-02T00:00:00.000000000Z', CAST('{}' AS BLOB));
		INSERT INTO rule_proposals (proposal_id, tenant_id, draft_id, diagnosis_code, kind_digest, intent, preset, risk_class, rule_json,
			proposal_digest, replay_json, evidence_cases, status, created_at, updated_at)
		VALUES ('rprop_1', 'default', 'rdraft_1', 'bearing_wear', 'sha256:aa', 'schedule_maintenance', 'bearing_wear', 'R1', CAST('{}' AS BLOB),
			'sha256:bb', CAST('{}' AS BLOB), 3, 'pending', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`)
	if err != nil {
		t.Fatal(err)
	}
	return db.DB
}

func TestTheDeskSignsEveryReadyAnswerThroughTheEndpoint(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &fakeEndpoint{key: public, resolved: map[string]bool{}}
	server := httptest.NewServer(endpoint)
	t.Cleanup(server.Close)
	db := pendingDatabase(t)
	desk := &Desk{Relay: Relay{BaseURL: server.URL, Token: "token", Approver: "campaign-approver", Key: private, Client: server.Client()},
		Approver: Careful{Coverage: 3, After: 4 * time.Hour}}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if answered, err := desk.Act(t.Context(), db, start); err != nil || answered != 0 {
		t.Fatalf("first look answered %d (%v), want none before four hours", answered, err)
	}
	if answered, err := desk.Act(t.Context(), db, start.Add(5*time.Hour)); err != nil || answered != 2 {
		t.Fatalf("later look answered %d (%v), want both", answered, err)
	}
	if !endpoint.resolved["appr_1"] || !endpoint.resolved["rprop_1"] {
		t.Fatalf("resolved = %v, want both approved", endpoint.resolved)
	}
	desk.Relay.Token = "wrong"
	if _, err := desk.Act(t.Context(), db, start); !errors.Is(err, errRelayRefused) {
		t.Fatalf("a refused request = %v, want the endpoint's refusal", err)
	}
}
