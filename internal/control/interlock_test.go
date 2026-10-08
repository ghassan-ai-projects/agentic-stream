package control_test

import (
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

// Trip needs no lease; clear needs the owner's lease in the same transaction.
func TestInterlockTripIsUnfencedAndClearIsOwnerFenced(t *testing.T) {
	t.Parallel()
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	tripped, err := runtimecontrol.TripInterlock(ctx, db, "operator stop")
	if err != nil || tripped.Status != "tripped" || tripped.Version != 2 {
		t.Fatalf("trip = %+v, %v", tripped, err)
	}
	stranger := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "operator", Lease: time.Minute}
	if _, err := stranger.ClearInterlock(ctx, "epoch-without-lease", "too early"); err == nil {
		t.Fatal("a clear without the owner lease was accepted")
	}
	if err := stranger.Claim(ctx, "epoch-operator"); err != nil {
		t.Fatal(err)
	}
	cleared, err := stranger.ClearInterlock(ctx, "epoch-operator", "inspected")
	if err != nil || cleared.Status != "ready" || cleared.Version != 3 {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	read, err := runtimecontrol.ReadInterlock(ctx, db)
	if err != nil || read != cleared {
		t.Fatalf("read = %+v, %v; want %+v", read, err, cleared)
	}
}
