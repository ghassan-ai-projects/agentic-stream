# DUP-024: Detached 5 s persistence idiom (five sites) and watch retry loop that copies storage retry

- Status: open
- Severity: low
- Verdict (finders): DIVERGED, REAL
- Themes: business rules, mechanisms
- Wave: 2
- Finder sources: R8, M7 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Add one detached bounded context helper and use it at the five sites; make watch expiry use `storage.RetrySQLiteBusy`. Fold the timer-and-select wait helpers only if they are exact copies.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R8: Hand-rolled infrastructure mechanisms: detach-and-bound persistence, and SQLite busy retry

- Verdict: REAL
- Shared meaning: (a) "finish persisting after the caller was cancelled, but bound it to 5s"; (b) "retry while SQLite reports writer contention".
- Sites (a): internal/evidence/internal/app/ledger_lifecycle.go:18 and :42 (`context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)`, both with the comment "Persist even when the caller was canceled"); internal/episodes/internal/app/executor.go:58 (same, 5s); internal/device/internal/app/session_state.go:14 (`reconciliationPersistTimeout = 5 * time.Second`) used at session_reconcile.go:58; cmd/agentic-stream/serve.go:175 (shutdown ctx, same 5s idiom).
- Sites (b): internal/storage/internal/domain/rules.go:13-17,29-39 + internal/storage/internal/store/sqlite.go:29-41 (`RetrySQLiteBusy`: 6 attempts, 25ms doubling up to 1s, cancellable wait); internal/watch/internal/domain/retry.go:7-10 (`ExpireAttempts = 3`, `ExpireBackoff = 250ms`) with its own loop and wait in internal/watch/internal/app/expire.go:34-56, gated by `store.IsContended` (internal/watch/internal/store/tx.go:46, a one-line alias of `storage.IsSQLiteBusy`). Engine already uses the shared helper (internal/engine/internal/store/tx.go:51-53).
- How they differ: (a) identical 5s everywhere, no shared name; (b) watch retries 3x at a flat 250ms where storage retries 6x with exponential backoff, for the same condition.
- Risk if left: changing the persistence grace period needs 5 edits; the watch expiry path keeps a different retry policy for the same SQLite error class.
- Proposed canonical owner: (a) `internal/sources` (time/context helpers; evidence/app, episodes/app and cmd already import it; device/app needs one edge) as `sources.DetachedContext(ctx) (context.Context, cancel)` with a `PersistGrace = 5*time.Second`. (b) `internal/storage` (watch/store and app already depend on it through store).
- Proposed fix: (a) add the helper and replace the 5 sites; (b) have watch `Expire` call `storage.RetrySQLiteBusy` (through its store) and delete retry.go and `awaitRetry`/`retryWhileContended`.
- Behaviour to preserve: values detach from cancellation (trace values kept); 5s bound; watch expiry semantics (idempotent update).
- Verification: evidence ledger, episodes runner, device session_reconcile tests; watch expire tests (none reference ExpireAttempts). New: a unit test for the helper (detached from cancel, bounded by deadline).

### Finder report M7: Bounded retry / wait-for-retry implemented four times; watch retry is a diverged copy of storage's

- Verdict: DIVERGED (watch), REAL (waits)
- Shared meaning: retry an operation on transient SQLite writer contention with a bounded, ctx-cancellable backoff.
- Sites:
  - internal/storage/internal/store/sqlite.go:29-53 `RetrySQLiteBusy` + `waitForSQLiteRetry`; constants in storage/internal/domain/rules.go:12-35 (6 attempts, 25 ms doubling to 1 s). Exposed as `storage.RetrySQLiteBusy`, used by engine/internal/store/tx.go:51 `RetryBusy` (engine/internal/app/apply.go:16).
  - internal/watch/internal/app/expire.go:34-60 `retryWhileContended` + `awaitRetry`; constants watch/internal/domain/retry.go:8-9 (3 attempts, fixed 250 ms); contention test `store.IsContended` = `storage.IsSQLiteBusy` (watch/internal/store/tx.go:46).
  - internal/executor/native/internal/app/loop.go:164-173 `waitProviderRetry` (timer + ctx select) for provider retries.
  - internal/ingress/internal/transport/server.go:98-113 `retryAccept` (timer + ctx select, 5 ms).
- How they differ: watch re-implements the busy retry loop with different attempts/backoff, different cancellation message ("expire watch conditions: wait for retry: ...") and a different final wrap; the DSN already sets busy_timeout(30000) (storage domain rules.go:23), so these loops are a second line of defence whose policy should be single. The three timer-plus-select waits are the same helper.
- Risk if left: tuning contention handling (attempts, jitter) needs edits in storage and watch; watch keeps retrying 3x250 ms while engine retries 6x up to 1 s for the same SQLITE_BUSY.
- Proposed canonical owner: `internal/storage` (already exports `RetrySQLiteBusy`, `IsSQLiteBusy`); watch already imports storage (allowedImports `internal/watch` and `internal/watch/internal/store`).
- Proposed fix: watch `Expire` calls `storage.RetrySQLiteBusy` through its store (delete `retryWhileContended`, `awaitRetry`, `ExpireAttempts/ExpireBackoff`, `IsContended`). Optionally add `sources`/`storage` helper `Sleep(ctx, d) error` for the timer+select used by `waitForSQLiteRetry`, `waitProviderRetry`.
- Behaviour to preserve: expiry runs once per second tick and must not hold the tick longer than its interval (pipeline.go:63-80 maintainWatches); error wrap `expire watch conditions` if tests match it; engine RetryBusy behaviour unchanged.
- Verification: watch expire tests, storage sqlite_retry_test.go. New test: watch expire survives 2 busy failures and gives up with the storage error after the shared attempt limit.

## Outcome

Not started.
