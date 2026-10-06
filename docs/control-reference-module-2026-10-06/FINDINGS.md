# Findings

## Mixed responsibilities

- `epoch_control.go` mixes the epoch-state decisions (`decisionEpochState`, `ordinaryEpochState`),
  the drain/kill transaction, cost release for unstarted episodes and four SQL statements.
- `runtime_owner.go` mixes the lease rules (default lease, validity) with SQL and time encoding.
- `costcontrol` mixes admission decisions (zero estimate under active control, kill switch,
  ceiling arithmetic, settlement idempotency) with SQL in the same functions.

## Package split

- `costcontrol` is layer 0 with no imports; `control` imports it for one call
  (`Settle` when a kill releases unstarted episodes). Everything that reserves cost also asks
  `control` whether the epoch is draining or killed (admission, episodes, runtime).
- `episodeledger` imports `costcontrol` only to settle abandoned attempts during recovery, which
  forbids the merge until that edge is a port.

## Dead and test-only code

`deadcode ./...` reports nothing for either package; every exported symbol is used in production.

## Leaking public surface

- `Controller struct{}` carries no state; the name hides that it is the episode cost ledger.
- `ErrReservationRejected`, `SetLimit`, `Ceilings`, `ApplyCeilings` lose their meaning without the
  `costcontrol.` prefix.
- `RuntimeOwner` and `EpochControl` are built as literals (about 70 sites). A missing database is
  reported on every call instead of at construction.

## Optional safety dependencies

- `RuntimeOwner`/`EpochControl` refuse an unconfigured instance at each call (fail-closed, not
  skipped). Recovery accepts a nil cost controller and silently skips cost release.

## Cross-module data access

- `unstartedEpisodeReservationsSQL` reads `episodes` (owned by `episodeledger`) joined with
  `cost_reservations`. Recorded as deferred: it needs an owner-provided read port.
- `epoch_control` is read by authority, admission and episodes only through control's methods.

## Smaller defects

- `time.Now()` defaults inside `RuntimeOwner.now` and `EpochControl.now` are clock reads in the
  persistence package.
- Errors are wrapped with `fmt.Errorf("%w", err)` no-ops in two places.
- The state strings `"draining"`/`"killed"` are repeated as literals in SQL, switch and setter.
