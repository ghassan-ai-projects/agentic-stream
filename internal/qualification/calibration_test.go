package qualification_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

func TestCalibrationActivationReplacesPriorArtifact(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	store := &qualification.CalibrationStore{DB: db}
	first := qualification.CalibrationArtifact{Domain: "motors", ModelRevision: "rev-1", ProfileDigest: "p", PromptSHA256: "s", DiagnosisCatalogSHA: "d", PolicyDigest: "pol"}
	second := first
	second.ModelRevision = "rev-2"

	assert := func(artifact qualification.CalibrationArtifact) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return store.AssertCalibration(ctx, tx, artifact) })
	}
	if err := assert(first); !errors.Is(err, qualification.ErrCalibrationMissing) {
		t.Fatalf("missing calibration = %v", err)
	}
	if err := store.Activate(ctx, first, "sha256-first-artifact-digest"); err != nil {
		t.Fatalf("activate first: %v", err)
	}
	if err := assert(first); err != nil {
		t.Fatalf("active calibration rejected: %v", err)
	}
	if err := store.Activate(ctx, second, "sha256-second-artifact-digest"); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	if err := assert(first); !errors.Is(err, qualification.ErrCalibrationMissing) {
		t.Fatalf("superseded calibration = %v, want ErrCalibrationMissing", err)
	}
	if err := assert(second); err != nil {
		t.Fatalf("replacement calibration rejected: %v", err)
	}
	if err := (&qualification.CalibrationStore{}).Activate(ctx, first, "x"); err == nil {
		t.Fatal("unconfigured store activated an artifact")
	}
}

func TestFailedCalibrationReplacementRetainsActiveArtifact(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	store := &qualification.CalibrationStore{DB: db}
	first := qualification.CalibrationArtifact{Domain: "motors", ModelRevision: "rev-1"}
	if err := store.Activate(ctx, first, "sharedprefix-first"); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ModelRevision = "rev-2"
	// Different digests sharing the artifact ID prefix fail after deactivation.
	if err := store.Activate(ctx, second, "sharedprefix-second"); err == nil {
		t.Fatal("artifact identity conflict was ignored")
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return store.AssertCalibration(ctx, tx, first) }); err != nil {
		t.Fatalf("failed replacement deactivated the prior artifact: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return store.AssertCalibration(ctx, tx, second) }); !errors.Is(err, qualification.ErrCalibrationMissing) {
		t.Fatalf("failed replacement became active: %v", err)
	}
}
