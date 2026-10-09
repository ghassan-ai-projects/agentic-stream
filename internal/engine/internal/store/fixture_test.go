package store

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

var compiledSpec = sync.OnceValues(func() (*spec.CompiledSpec, error) {
	return spec.CompileFile(context.Background(), "../../../../examples/predictive-maintenance/predictive-maintenance.situation.yaml")
})

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func openStore(t *testing.T) Store {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	compiled, err := compiledSpec()
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, allowOwner, "epoch", "tenant", compiled.Digest)
	if err := s.SaveDeployment(t.Context(), compiled); err != nil {
		t.Fatal(err)
	}
	return s
}

func inTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := s.WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func publishedVersion(version int) situations.Version {
	return situations.Version{SituationID: "sit-1", Version: version, PreviousVersion: version - 1, Type: "bearing", EntityType: "motor", EntityID: "m1",
		Phase: "watch", EventHorizon: testNow, Watermark: testNow, Completeness: "on_time", Severity: 1, Confidence: 0.5,
		SnapshotJSON: []byte(`{"v":1}`), SnapshotSHA256: "sha256:" + string(repeat('0', 64)), Evidence: []string{"evt-1"}}
}

func repeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func publishWrite() domain.SituationWrite {
	return domain.SituationWrite{OccurrenceID: "occ-1", FirstEventTime: testNow, StateJSON: []byte(`{}`), StateDigest: make([]byte, 32)}
}

func publish(t *testing.T, s Store, version situations.Version) {
	t.Helper()
	lineage, err := domain.NewLineage(version.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	inTx(t, s, func(tx *Tx) error {
		if err := tx.RecordLineage(t.Context(), lineage, testNow); err != nil {
			return err
		}
		if err := tx.UpsertSituation(t.Context(), 0, version, publishWrite(), testNow); err != nil {
			return err
		}
		return tx.InsertSituationVersion(t.Context(), version, lineage.ID, testNow)
	})
}

func operatorState(keys ...string) *operators.PartitionState {
	blobs := make(map[string]*operators.OperatorStateBlob, len(keys))
	for _, key := range keys {
		blobs[key] = &operators.OperatorStateBlob{}
	}
	return &operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{"op": blobs}}
}

// failingTx runs work in a transaction whose table was dropped, rolls the
// transaction back, and returns the error work reported.
func failingTx(t *testing.T, s Store, table string, work func(context.Context, *Tx) error) error {
	t.Helper()
	var reported error
	_ = s.WithTx(t.Context(), func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(t.Context(), "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
		reported = work(t.Context(), tx)
		return reported
	})
	return reported
}

type txFailure struct {
	name, table string
	work        func(context.Context, *Tx) error
	want        string
}

func checkTxFailures(t *testing.T, cases []txFailure) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := failingTx(t, openStore(t), tc.table, tc.work); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
