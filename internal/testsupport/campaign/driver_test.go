package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const fakeStreamsim = `#!/bin/sh
case "$1" in
plantwire)
	trap 'exit 0' INT
	while [ $# -gt 0 ]; do [ "$1" = "--out" ] && out="$2"; shift; done
	mkdir -p "$out" && printf '{"run_id":"run-1"}' > "$out/run.json"
	echo ready > "$(dirname "$0")/ready"
	while :; do sleep 0.05; done;;
score) printf '{"graded":true}';;
esac
`

func fakeSimulator(t *testing.T) Simulator {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("streamsim", []byte(fakeStreamsim), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Simulator{Binary: filepath.Join(dir, "streamsim"), Repository: dir}
}

func awaitSessionReady(simulator Simulator) error {
	ready, err := os.Open(filepath.Join(simulator.Repository, "ready"))
	if err != nil {
		return err
	}
	_, err = io.ReadAll(ready)
	return errors.Join(err, ready.Close())
}

func ingest(ctx context.Context, db *sql.DB, eventTimes ...string) error {
	for i, eventTime := range eventTimes {
		_, err := db.ExecContext(ctx, `INSERT INTO event_log (tenant_id, partition_id, event_id, event_type, schema_version, source, partition_key,
			entity_type, entity_id, event_time, ingested_at, classification, quality_json, payload_json, payload_sha256, created_at)
			VALUES ('default', 0, ?1, 'pump.vibration.observed', '1.1', 'plantwire', 'site-a:pump-3', 'pump', 'site-a:pump-3', ?2, ?3,
				'internal', CAST('{}' AS BLOB), CAST('{}' AS BLOB), zeroblob(32), ?3)`, "evt-"+eventTime+string(rune('a'+i)), eventTime, kernel.FormatTime(time.Now()))
		if err != nil {
			return err
		}
	}
	return nil
}

func TestTheDriverPlaysARunToItsSimulatedEndAndGradesIt(t *testing.T) {
	t.Parallel()
	loaded := writeCampaign(t)
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	stopped := 0
	simulator := fakeSimulator(t)
	driver := Driver{Campaign: loaded, Arm: Arm{Name: "A", Approver: ApproveAll{}}, Simulator: simulator, Dir: t.TempDir(),
		Serve: func(ctx context.Context, _ Run) (Consumer, error) {
			go func() {
				if awaitSessionReady(simulator) == nil {
					_ = ingest(context.WithoutCancel(ctx), db, "2026-01-01T00:00:00.000000000Z", "2026-01-01T03:00:00.000000000Z")
				}
			}()
			return Consumer{DB: db, HostSocket: "/tmp/as/host.sock", Desk: &Desk{Approver: ApproveAll{}}, Stop: func() error { stopped++; return nil }}, nil
		}}
	report, err := driver.Drive(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || string(report.Runs[0].Grade) != `{"graded":true}` || stopped != 1 {
		t.Fatalf("report = %+v, consumer stopped %d times; want one graded run and one stop", report, stopped)
	}
	verdictJSON, err := readIn(filepath.Join(driver.Dir, "run-0001"), verdictFile)
	if err != nil || !strings.Contains(string(verdictJSON), `"run_id":"run-1"`) || !strings.Contains(string(verdictJSON), `"arm":"A"`) {
		t.Fatalf("verdict = %s (%v)", verdictJSON, err)
	}
	var written Report
	if data, err := readIn(driver.Dir, reportFile); err != nil || json.Unmarshal(data, &written) != nil || written.Phases["learn"].ModelEpisodes != 0 {
		t.Fatalf("written report = %+v (%v)", written, err)
	}
}

func TestTheDriverStopsWhenTheConsumerCannotStart(t *testing.T) {
	t.Parallel()
	driver := Driver{Campaign: writeCampaign(t), Arm: Arm{Name: "A"}, Simulator: fakeSimulator(t), Dir: t.TempDir(),
		Serve: func(context.Context, Run) (Consumer, error) { return Consumer{}, os.ErrNotExist }}
	if _, err := driver.Drive(t.Context(), 1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Drive without a consumer = %v, want the consumer's error", err)
	}
}

func TestLedgerSharesAndPercentiles(t *testing.T) {
	t.Parallel()
	if share := (Ledger{ModelEpisodes: 1, RuleEpisodes: 3}).RuleShare(); share != 0.75 {
		t.Fatalf("rule share = %v, want 0.75", share)
	}
	if share := (Ledger{}).RuleShare(); share != 0 {
		t.Fatalf("empty rule share = %v", share)
	}
	if p95 := percentile95([]float64{5, 1, 3, 2, 4}); p95 != 5 {
		t.Fatalf("p95 = %v, want 5", p95)
	}
	if believedOutcome("reconciling") != "unknown" || instanceOf("site-a:pump-3") != "site-a/pump-3" {
		t.Fatal("verdict vocabulary changed")
	}
}
