package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestEpisodeStateIsReadFromTheLedgerForEveryLifecycleAndBinding(t *testing.T) {
	t.Parallel()
	base := ledgerCall("call-1")
	lowerFence, higherFence, otherAttempt := base, base, base
	lowerFence.Fence, higherFence.Fence, otherAttempt.AttemptID = 0, 2, "attempt-2"
	calls := map[string]domain.Call{"current": base, "lower fence": lowerFence, "higher fence": higherFence, "other attempt": otherAttempt}
	lifecycles := map[episodeledger.LifecycleStatus]struct{ closed, running bool }{
		episodeledger.LifecycleAdmitted: {false, false}, episodeledger.LifecycleRunning: {false, true},
		episodeledger.LifecycleConcluded: {true, false}, episodeledger.LifecycleClosed: {true, false},
		episodeledger.LifecycleSuperseded: {true, false}, episodeledger.LifecycleExpired: {true, false},
		episodeledger.LifecycleAbandoned: {true, false},
	}
	acceptedAtReservation := map[string]bool{"admitted/current": true, "running/current": true}
	acceptedAtCompletion := map[string]bool{"running/current": true}
	for lifecycle, want := range lifecycles {
		t.Run(string(lifecycle), func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
				t.Fatal(err)
			}
			for name, call := range calls {
				label := string(lifecycle) + "/" + name
				wantState := domain.EpisodeState{Current: name == "current", Closed: want.closed, Running: want.running}
				mustInTx(t, s, func(tx *Tx) error {
					live, err := tx.LiveEpisode(t.Context(), call)
					completion, completionErr := tx.CompletionEpisode(t.Context(), reservationKey(call))
					if err != nil || completionErr != nil || live != wantState || completion != wantState {
						t.Errorf("%s: live=%+v completion=%+v errors=%v %v, want %+v", label, live, completion, err, completionErr, wantState)
					}
					if accepted := domain.CheckLiveEpisode(live) == nil; accepted != acceptedAtReservation[label] {
						t.Errorf("%s: reservation accepted = %v", label, accepted)
					}
					if accepted := domain.CheckCompletionEpisode(completion) == nil; accepted != acceptedAtCompletion[label] {
						t.Errorf("%s: completion accepted = %v", label, accepted)
					}
					return nil
				})
			}
		})
	}
}

func TestEpisodeOfAnotherTenantOrWithoutARowIsNeverLoadedAtReservation(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	tests := map[string]func(*domain.Call){
		"unknown episode": func(c *domain.Call) { c.EpisodeID = "episode-missing" },
		"another tenant":  func(c *domain.Call) { c.TenantID = "tenant-2" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			call := ledgerCall("call-1")
			mutate(&call)
			mustInTx(t, s, func(tx *Tx) error {
				if state, err := tx.LiveEpisode(t.Context(), call); err == nil || state.Current {
					t.Errorf("LiveEpisode = %+v, err %v, want a refusal", state, err)
				}
				return nil
			})
		})
	}
}

func TestEpisodeWithoutAnAttemptOrWithoutARowIsNeverCurrent(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET current_attempt_id = NULL, current_fence = 0"); err != nil {
		t.Fatal(err)
	}
	call := ledgerCall("call-1")
	call.AttemptID, call.Fence = "", 0
	mustInTx(t, s, func(tx *Tx) error {
		live, err := tx.LiveEpisode(t.Context(), call)
		if err != nil || live.Current {
			t.Errorf("episode without an attempt: live=%+v err=%v", live, err)
		}
		missing := domain.ReservationKey{TenantID: call.TenantID, EpisodeID: "episode-missing", AttemptID: "attempt-1", Fence: 1}
		completion, err := tx.CompletionEpisode(t.Context(), missing)
		if err != nil || completion.Current || completion.Running || domain.CheckCompletionEpisode(completion) == nil {
			t.Errorf("unknown episode at completion: %+v err=%v", completion, err)
		}
		return nil
	})
}

func TestAttemptIsInFlightOnlyWhileDispatchedOrRunning(t *testing.T) {
	t.Parallel()
	call := ledgerCall("call-1")
	for attempt, want := range map[episodeledger.AttemptStatus]bool{
		episodeledger.AttemptDispatched: true, episodeledger.AttemptRunning: true, episodeledger.AttemptCancelling: false,
		episodeledger.AttemptProduced: false, episodeledger.AttemptDeclined: false, episodeledger.AttemptCancelled: false,
		episodeledger.AttemptFailed: false, episodeledger.AttemptTimedOut: false, episodeledger.AttemptAbandoned: false,
	} {
		t.Run(string(attempt), func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			if _, err := db.ExecContext(t.Context(), "UPDATE episode_attempts SET status = ?", string(attempt)); err != nil {
				t.Fatal(err)
			}
			mustInTx(t, s, func(tx *Tx) error {
				live, err := tx.LiveAttempt(t.Context(), call)
				completion, completionErr := tx.CompletionAttempt(t.Context(), reservationKey(call))
				if err != nil || completionErr != nil || live != want || completion != want {
					t.Errorf("live=%v completion=%v errors=%v %v, want %v", live, completion, err, completionErr, want)
				}
				return nil
			})
		})
	}
}

func TestAnAttemptThatIsNotTheFencedOneIsRefusedWhenRead(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	tests := map[string]func(*domain.Call){
		"unknown attempt": func(c *domain.Call) { c.AttemptID = "attempt-9" },
		"lower fence":     func(c *domain.Call) { c.Fence = 0 },
		"higher fence":    func(c *domain.Call) { c.Fence = 2 },
		"unknown episode": func(c *domain.Call) { c.EpisodeID = "episode-9" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			call := ledgerCall("call-1")
			change(&call)
			mustInTx(t, s, func(tx *Tx) error {
				if inFlight, err := tx.LiveAttempt(t.Context(), call); err == nil || inFlight {
					t.Errorf("LiveAttempt = %v, %v", inFlight, err)
				}
				if inFlight, err := tx.CompletionAttempt(t.Context(), reservationKey(call)); err == nil || inFlight {
					t.Errorf("CompletionAttempt = %v, %v", inFlight, err)
				}
				return nil
			})
		})
	}
}
