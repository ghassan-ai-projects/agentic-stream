# A-044 · `internal/eventlog/quarantine.go`

LOC: 209 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported quarantine operation has a production caller or is explicitly deferred.
- Quarantined data never reaches `event_log` without full re-validation.
- Retry bounding, hash-conflict detection, and gap records behave as documented.
- Errors wrapped/sentinel; comparisons use `errors.Is`.

## Findings
- **[MED] F1. ReadQuarantine/ReleaseQuarantine/RedriveQuarantine have no production caller** — `internal/eventlog/quarantine.go:117-197`. Grep across `cmd/` and `internal/` (non-test) finds zero callers; only `quarantine_test.go` uses `ReleaseQuarantine`. The release/redrive half of the quarantine lifecycle is unwired: an operator has no CLI/API path to re-drive a rejected-or-releasable event, so quarantined evidence can only pile up. Wire it into `cmd/agentic-stream` (inspect/release/redrive subcommands) or delete the surface until it exists.
- **[LOW] F2. Rejection flip lags one delivery behind the attempt cap** — `internal/eventlog/quarantine.go:55-60`. `status = CASE WHEN attempt_count >= 10 ...` reads the pre-update value while `MIN(attempt_count + 1, 10)` writes, so a row reaches `attempt_count = 10` while still `quarantined` and only flips to `rejected` on an 11th delivery. `quarantine_test.go` pins the current behavior; if the intent is "rejected at the 10th attempt", compare against `attempt_count + 1` or update status from the new value.
- **[LOW] F3. Overflow gap row uses placeholder positions** — `internal/eventlog/quarantine.go:82`. The `quarantine_retry_exhausted` gap is inserted with `partition_id = 0, from_position = 0, to_position = 0`, which contradicts `event_gaps` semantics of a positional discontinuity (`RecordGap` validates `to >= from` as real positions elsewhere). The reason code carries the meaning; consider a dedicated marker or document the placeholder.
- **[LOW] F4. Direct sql.ErrNoRows comparison** — `internal/eventlog/quarantine.go:47`. `existingErr != sql.ErrNoRows` instead of `errors.Is`; same pattern the repo elsewhere avoids.

## Checked, not an issue
- P1: quarantine runs in one `WithTx`; hash conflicts are detected twice (pre-SELECT digest compare and `RowsAffected == 0` fallback on the guarded upsert) and fail closed with `event_id_hash_conflict` + error return; all errors wrapped `%w`.
- P2: quarantined payloads are stored as opaque data; `RedriveQuarantine` re-validates the full envelope and the registered schema inside the same transaction before `appendOne`, and marks `redriven_at` so it cannot redrive twice; raw malformed JSON is preserved with a stable payload-digest identity, never executed.
- P4: matches `migrations/016_event_schemas_quarantine.sql` + 018 (`redriven_at`); status/attempts enforced by table CHECKs plus `MIN(attempt_count + 1, 10)`.
- P6: `quarantine_test.go` pins bounded retries, rejection + single overflow gap, hash-conflict rejection, and release; `RecordGap` validated and tested.
