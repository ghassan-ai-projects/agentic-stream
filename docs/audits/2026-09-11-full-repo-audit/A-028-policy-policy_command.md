# A-028 · `internal/policy/policy_command.go`

LOC: 300 · Audit date: 2026-09-11 · Verdict: FINDINGS

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
- P2: command creation is idempotent (`existingCommand` pre-check + `ON CONFLICT(intent_id) DO NOTHING` + deterministic idempotency key); command is canonical-JSON digested before insert; approval nonce is derived and single-use; outbox insert is atomic with the command.
- P3: `approvalNotificationData`'s local map fill (226-228) is ugly but harmless; no unused exported symbols in the file.
- P4: policy plane only prepares commands/outbox rows; it does not dispatch — separation holds.
- P5: canonical JSON used for command document and digest; `formatTime` helper shared via `policy_helpers.go`.
- P6: `policy_test.go` (interlock denial) and `rate_limit_test.go` cover the paths; `go test ./internal/policy/` passes.
- P7: command IDs from `ids.Generator`, idempotency key derived from stable identities; bucket key is UTC hour.
