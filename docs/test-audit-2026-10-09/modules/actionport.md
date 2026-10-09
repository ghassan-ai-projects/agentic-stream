# actionport

Status: done
Round: 12

Audited in round 12 with [actions](actions.md). Production code is unchanged.
The module is a contract (effector port, `UnknownOutcomeError`, command status
vocabulary); its tests now sit at the layer that owns each rule.

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/actionport` (facade) | 100.0% | 100.0% | 1.4 s | 1.1 s | 3 / 0 | 2 / 0 |
| `internal/actionport/internal/domain` | 80.0% | 100.0% | 1.4 s | 1.1 s | 2 / 0 | 4 / 7 |

Hygiene lint: 0 findings before and after. Repository `golangci-lint`: 0 issues.

## Findings and changes

### Removed
- The zero-value assertion in `TestFacadeClassifiesUnknownOutcomes`
  (`command.CommandID != "" || effect.VerificationPending`): it tested that a Go zero value
  is zero (T10).

### Renamed or moved
- `TestFacadeClassifiesUnknownOutcomes` is `TestTheFacadeClassifiesAnUnknownOutcomeAndKeepsItsCause`
  and now wraps the error to prove it is recognized through wrapping.
- `TestUnresolvedCommandStatusesAreExactlyTheStatesAwaitingReconciliation` and
  `TestUnresolvedCommandStatusesCannotBeMutatedByCallers` moved from the facade to
  `internal/domain/status_test.go` (T2: the rule lives in domain); the facade keeps one test,
  `TestTheFacadeReportsTheStatesAwaitingReconciliation`, for its delegation.

### Improved
- T6: the status matrix is a parallel subtest per status.

### Added
- Domain coverage of `UnresolvedCommandStatuses` and `IsUnresolvedCommandStatus` (80% to 100%).

### Speed
- Nothing was slow.

## Production code touched
- none.

## Invariants proven here
- 8: `TestUnknownOutcomePreservesCauseAndRequiresReconciliation` (domain) and the unresolved
  status set (`TestUnresolvedCommandStatusesAreExactlyTheStatesAwaitingReconciliation`) are what
  actions, authority and the soak report share to decide that an effect must be reconciled, not
  retried.

## Open items
- none.
