# X10 — The global engine run resumes from the applied position

Status: done · Decision: **fix (found while deleting U04)** · Priority: P0 (experiment soak) · Size: S

## Finding

`engine.RunGlobal` started reading the event log at position 0 on every call
and relied on the event inbox to skip records it had already applied. For each
record ever ingested, every run loaded the partition checkpoint, ran the due
timer scan for all partitions, opened a write transaction, asserted the runtime
owner and checked the inbox. The live pipeline runs it for every ingested event
and, since X08, on every clock tick, so the cost of each run grew with the
whole log. The joined runs had 61 events and never showed it; an 8-hour bench
soak (G5) would slow down steadily. Found when the U04 test port showed a
second run reporting already-applied records as processed again.

## Decision and reasoning

Resume from the durable applied position: the highest partition checkpoint
(`AppliedThrough`). Records are applied in log-position order and SQLite
serializes writers, so positions commit in order: nothing at or below that
position is left to apply. A failed record leaves the checkpoint below it, so
the next run retries it. The inbox stays as the duplicate guard.

## Evidence

`TestGlobalRunResumesAfterTheAppliedPosition`: after 50 applied records, an idle
run visits none of them, and the next run applies only the new record.
`TestGlobalRunFailurePreservesProgressAndInboxDeduplication` now expects the
resumed run to apply only the failed record. Golden replay hashes are unchanged.
