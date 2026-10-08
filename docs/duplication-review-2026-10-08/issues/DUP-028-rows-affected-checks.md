# DUP-028: The exactly-one-row check after a fenced write has four error policies

- Status: open
- Severity: low
- Verdict (finders): DIVERGED
- Themes: mechanisms
- Wave: 3
- Finder sources: M11 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Mild. Decide one policy per intent (must-affect-one vs report-bool) and ensure RowsAffected errors are never silently swallowed on fenced writes.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M11: "Exactly one row changed" check after a fenced write, with four error policies

- Verdict: DIVERGED (mild)
- Shared meaning: a lease/fence/ownership-predicated UPDATE that affects other than one row means the fence was lost; the caller must fail closed.
- Sites: evidence/internal/store/writes.go:31 and :43 (`n, _ := RowsAffected(); n != 1`, error ignored); notify/internal/store/append.go:81 and :101 (`affected, _ :=`); actions/internal/store/leases.go:46-49 (`count, err ...; return err == nil && count == 1, nil`, driver error swallowed); control/internal/store/owner.go:65-69 + domain `CheckOwnerMutation` (error propagated, count checked in domain); interlock/internal/store/store.go:52 (`err != nil || count != 1` -> "row was not updated"); policy/internal/store/principal_writes.go:45 (`err != nil || count != 1` -> "belongs to another tenant", error cause lost); engine/internal/store/situations.go:109-118 (`affected == 0`, error wrapped); control/internal/store/cost_limits.go:54, episodeledger/internal/store/attempt.go:63, scheduler_queue.go:21,53, watch/internal/store/watches.go:81, eventlog quarantine.go:33,84, events.go:51, policy command_writes.go:30,56 (all propagate). The shared helper `storage.RowsAffected` (storage/storage.go:48) swallows the error to 0 and is used elsewhere.
- How they differ: four behaviours for `RowsAffected()` errors: ignore, swallow into 0/false, replace by a domain message that hides the driver error, propagate. All fail closed, but the cause is lost in some.
- Risk if left: low; diagnosability and consistency only. One new fenced write copies whichever shape was nearest.
- Proposed canonical owner: `internal/storage` (`storage.RowsAffected` already lives there).
- Proposed fix: add `storage.RequireOneRow(result, err) error` (or `ExactlyOne`) returning a sentinel `storage.ErrNoRowMatched`; callers wrap with their own text. Adopt in evidence, notify, interlock, policy principals, actions lease, control owner first.
- Behaviour to preserve: the exact messages tests assert ("evidence call reservation is no longer owned", "runtime interlock row was not updated", "principal ... belongs to another tenant", "rollback duplicate cursor lost race").
- Verification: evidence ledger tests, interlock tests, notify cursor tests; new storage unit test for the helper.

## Outcome

Not started.
