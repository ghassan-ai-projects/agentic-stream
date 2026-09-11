# A-074 · `internal/actions/dispatcher_test.go`

LOC: 572 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every dispatcher error path that mutates durable state has a test asserting the resulting ledger state.
- The two effector fakes do not duplicate the unknown-outcome Dispatch branch.

## Findings
- **[MED] F1. Lease-failure path untested package-wide** — `internal/actions/dispatcher_test.go` (whole file) vs `internal/actions/dispatcher.go:369-476` (`lease` steal/conflict branch) and `dispatcher.go:808-828` (`markLeaseFailure`, `finishOutboxOnly`). `dispatcher_test.go` is the only test in the package driving `DispatchOnce`/`ReconcileUnknown`, and every scenario starts from an unleased pending command; the behavior when a lease is held/expired by another owner (skip vs steal, `markLeaseFailure` ledger writes) has zero coverage. Add a fixture variant that pre-inserts an expired and an active lease and asserts both outcomes.
- **[LOW] F2. Two effector fakes duplicate the unknown-outcome branch** — `internal/actions/dispatcher_test.go:40-46` and `:79-88`. `verifyingEffector.Dispatch` and `recordingEffector.Dispatch` are the same logic modulo `VerificationPending`; one configurable fake (fields for `unknown`, `verificationPending`, `verify`) removes the copy and the drift risk between them.
- **[LOW] F3. Wall-clock bound with 20x slack** — `:343-350`. The 50ms lease is asserted with a 1s elapsed ceiling; acceptable for CI jitter but the slack is wide enough to miss a 10x lease-regression. Tighten toward ~5x or assert the recorded lease timestamps instead of elapsed wall time.
- **[LOW] F4. `context.Background()` throughout** — no `t.Context()` usage despite repo style; `db.Close()` via `defer` instead of `t.Cleanup` in all nine tests.

## Checked, not an issue
- T1: assertions are exact ledger states (command/outbox/outcome/reconciliation/verification statuses), notification payload fields including digest hex and reconciliation versions, call counts, and negative controls (untyped reconciliation evidence rejected, no redispatch after delivery); no tautologies, no error-string-only assertions.
- T2: deterministic — the bounded-verification test blocks on `ctx.Done()` driven by the dispatcher's own lease cancellation, no sleeps, `db.SetMaxOpenConns(1)` prevents SQLite contention.
- T3: shared fixture `openActionFixture` plus `readNotification`/`readNotificationData`/`assertOutcomeNotificationAuthority` helpers keep tests thin; scenarios are distinct enough that table-driving would obscure them.
- T4: covers success idempotency, unknown-outcome no-retry + typed-evidence reconciliation, verification-pending holding state, device verification success/failure/unknown reconciliation, interlock refusal, and effector-side interlock recheck — strong behavioral spread.
- T5: fixture is written once and reused by all nine tests; no copy-pasted SQL blocks.
