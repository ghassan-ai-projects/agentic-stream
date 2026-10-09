package composition

import (
	"database/sql"
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNoOwnerBoundIsDecidedOnceByTheOwnershipCheck(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	unowned := runtimeOwnershipCheck(PipelineConfig{})
	owned := runtimeOwnershipCheck(PipelineConfig{Owner: &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance"}, OwnerEpoch: "epoch"})
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := unowned(t.Context(), tx, ""); err != nil {
			t.Errorf("unowned check refused: %v", err)
		}
		if err := owned(t.Context(), tx, "epoch"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
			t.Errorf("owned check without a lease = %v, want the owner assertion's refusal", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineRequiresOwnerAndEpochTogether(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance"}
	for name, cfg := range map[string]PipelineConfig{
		"owner without an epoch": {Owner: owner},
		"epoch without an owner": {OwnerEpoch: "epoch"},
	} {
		if err := validateOwnership(cfg); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for name, cfg := range map[string]PipelineConfig{
		"both":    {Owner: owner, OwnerEpoch: "epoch"},
		"neither": {},
	} {
		if err := validateOwnership(cfg); err != nil {
			t.Errorf("%s refused: %v", name, err)
		}
	}
}
