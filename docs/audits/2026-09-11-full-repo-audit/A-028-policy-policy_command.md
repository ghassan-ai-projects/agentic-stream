# A-028 · `internal/policy/policy_command.go`

LOC: 300 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- An infrastructure failure is never recorded as a durable policy denial; only a genuine policy verdict may set a terminal intent status.
- Rate-limit accounting counts dispatches, not denials.
- One lookup per concept: command-identity reads are not duplicated with divergent error semantics.

## Findings
- **[MED] F1. Interlock failure is conflated with a durable `interlock_not_ready` denial** — `internal/policy/policy_command.go:19-22,63-71`. Any error from `interlock.Assert` — including a transient SQLite/infra failure — is discarded and converted to `finish(..., "denied", "interlock_not_ready")`, permanently setting `policy_status='denied'` on the intent with the underlying cause neither wrapped, logged, nor recorded. A momentary read failure denies the intent forever and breaks the error/denial distinction the rest of the plane maintains. Fix: classify the assertion (interlock says "not ready" vs. the check itself failed); return genuine failures as errors so the evaluation is retried, and record the concrete cause in the audit reason when denying.
- **[LOW] F2. Rate-limited denials consume rate-limit budget** — `internal/policy/policy_command.go:34-42` with `internal/policy/policy_store.go:111-124`. `dispatchWithinLimit` increments the counter before the over-limit decision; when the answer is "over limit" the denial path commits, so denied attempts permanently consume the hourly budget and can lock out later legitimate dispatches. Conservative, but semantically wrong. Fix: decrement on the denial path or only count evaluations that actually insert a command.
- **[LOW] F3. `existingCommand` and `storedCommandID` are near-duplicates** — `internal/policy/policy_command.go:73-83,134-140`. Both run `SELECT command_id FROM commands WHERE intent_id = ?`; they differ only in `ErrNoRows` handling, and `storedCommandID`'s not-found case is unreachable right after `insertCommand`. Keep one helper with explicit `ErrNoRows` semantics.

## Checked, not an issue
- P1: all other errors wrapped with `%w`; contexts honored; single-tx mutations, no races (serial per partition).
- P2: command creation is idempotent (`existingCommandID` pre-check + `ON CONFLICT(intent_id) DO NOTHING` + deterministic idempotency key); command is canonical-JSON digested before insert; approval nonce is derived and single-use; outbox insert is atomic with the command.
- P3: `approvalNotificationData`'s local map fill (226-228) is ugly but harmless; no unused exported symbols in the file.
- P4: policy plane only prepares commands/outbox rows; it does not dispatch — separation holds.
- P5: canonical JSON used for command document and digest; `formatTime` helper shared via `policy_helpers.go`.
- P6: `policy_test.go`, `rate_limit_test.go`, and `policy_command_test.go` cover the success and failure paths; `go test ./internal/policy/` passes.
- P7: command IDs from `ids.Generator`, idempotency key derived from stable identities; bucket key is UTC hour.

## Resolution (2026-09-12) — FIXED

- **F1 fixed:** `approveAutomatic` classifies only `interlock.ErrTripped` as the genuine readiness denial. Other assertion failures are returned as errors, so the caller's transaction rolls back and the intent remains retryable. The stable `Result.Reason` remains `interlock_not_ready`; the policy audit row records the wrapped interlock cause through the separate audit-reason path. `TestGatewayPropagatesInterlockInfrastructureFailure` proves that an infrastructure error does not persist an intent status, audit row, or command, while `TestGatewayRecordsInterlockDenialCauseWithoutChangingStableReason` proves the denial and cause evidence.
- **F2 fixed:** command insertion is now identified before rate accounting. A new command claims a slot with an atomic conditional upsert only while the bucket is below its limit; an over-limit transaction removes its un-dispatched prepared command and records the denial without incrementing the bucket. `TestRateLimitDenialDoesNotConsumeDispatchBudget` proves the count stays unchanged and no command/outbox row remains.
- **F3 fixed:** `existingCommandID` is the single command-identity lookup with explicit `sql.ErrNoRows` semantics. `insertCommand` returns whether it inserted; successful inserts use the already-known command ID, while the conflict path reuses the same helper. The divergent `storedCommandID` helper was removed.

Focused evidence for this resolution:

- `go test -race -count=1 ./internal/policy` — PASS
- `go vet ./internal/policy` — PASS
- `git diff --check` — PASS

All three bar lines are satisfied: infrastructure failures remain retryable, rate-limit accounting represents dispatchable commands only, and command identity has one lookup contract.
