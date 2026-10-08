# U20 — Recorded replay mode

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U02

## Finding

Recorded replay re-runs the stream deterministically, rebuilds the episode
worklist, and checks every replayed episode against the worker result recorded
in a durable ledger, without calling a worker. The domain rules, matching and
validation exist and are tested (`RunMode`, `applyRecorded`,
`IndexRecordedEntries`, `VerifyRecordedEntry`, `ValidateRecordedSnapshot`,
`MatchRecordedMetadata`, …, plus the shared worker-aware plumbing: 32 symbols).
There is **no production `RecordedLedger`**: the only implementations are test
doubles, and the CLI exposes only deterministic `run`.

The MVP list requires "reproduce the accepted decision with recorded cognition";
IMPLEMENTATION_PLAN M3.5 requires "recorded cognition reproduces accepted
Decision".

## Decision and reasoning

Complete it. It is an MVP acceptance item, the hard part (verification rules) is
done, and it is the only way to show that a production decision was made on
exactly the Situation the stream reproduces.

Build two things:

1. **A read-only `RecordedLedgerForReplay` over a source runtime database.**
   Open the source with SQLite `mode=ro`; read decisions joined with their
   accepted worker attempt (attempt id, fence, provenance digest, manifest
   digest) for the replay worklist's episode keys. Refuse when the source
   deployment's spec digest differs from the replay spec. Design §16.2 forbids
   *sharing* scheduler rows, outbox, tokens and credentials; a read-only read
   of decisions shares none of them. The adapter lives in
   `replay/internal/transport` (it is an input source, like the trace file), and
   its SQL reads only tables the episodes and episodeledger modules already
   expose. If the SQL-ownership gate objects, add an episodes read port
   (FOLLOW_UPS #5 pattern).
2. **The CLI:**

   ```text
   agentic-stream run --spec <spec> --trace <trace> --mode recorded --source-db <runtime.db>
   ```

   Prints the deterministic result plus matched/mismatched episodes; exits
   non-zero on any mismatch or on a recorded entry with no replayed episode.

The alternative source, an exported run directory (`export-run`), is cleaner
for portability but lacks attempt provenance today. Adding it changes the
artifact format; do it only if a consumer needs offline recorded replay.

## Done when

- End-to-end test: `run-live` with the deterministic native executor writes a
  runtime DB; recorded replay of the same trace and spec against it passes; a
  tampered decision digest, a spec mismatch and a missing entry each fail with
  their own reasons.
- The recorded-mode symbols and the shared mode plumbing are reachable from
  `main`.
- `documentation/design/replay-and-shadow.md` "CLI reality" section is replaced
  by the command.
