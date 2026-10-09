package store

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func TestCommandBindingsAreReadBackWithOrWithoutADigest(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	digest := "sha256:abababababababababababababababababababababababababababababababab"
	withDigest := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: owner, CommandDigest: digest}
	withoutDigest := withDigest
	withoutDigest.CommandID, withoutDigest.CommandDigest = "cmd-2", ""
	work(t, s, func(tx *Tx) error { return tx.InsertBinding(t.Context(), withDigest, testNow) })
	work(t, s, func(tx *Tx) error { return tx.InsertBinding(t.Context(), withoutDigest, testNow) })
	inCallerTx := func(fn func(*Tx) error) {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return fn(Join(tx)) }); err != nil {
			t.Fatal(err)
		}
	}
	inCallerTx(func(tx *Tx) error {
		for _, want := range []domain.CommandBinding{withDigest, withoutDigest} {
			if got, err := tx.LoadBinding(t.Context(), want.CommandID); err != nil || *got != want {
				t.Fatalf("binding = %+v, %v; want %+v", got, err, want)
			}
		}
		if got, err := tx.LoadBinding(t.Context(), "unbound"); err != nil || got != nil {
			t.Fatalf("unbound command = %+v, %v", got, err)
		}
		return nil
	})
	invalid := withDigest
	invalid.CommandID, invalid.CommandDigest = "cmd-3", "bad"
	if err := s.InTx(t.Context(), func(tx *Tx) error { return tx.InsertBinding(t.Context(), invalid, testNow) }); err == nil {
		t.Fatal("invalid digest was stored")
	}
}

func TestACommandIsBoundToADeviceOnce(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	binding := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: owner}
	work(t, s, func(tx *Tx) error { return tx.InsertBinding(t.Context(), binding, testNow) })

	err := s.InTx(t.Context(), func(tx *Tx) error { return tx.InsertBinding(t.Context(), binding, testNow) })
	if err == nil || !strings.Contains(err.Error(), "record command binding") {
		t.Fatalf("second binding of the same command = %v, want a refusal", err)
	}
}
