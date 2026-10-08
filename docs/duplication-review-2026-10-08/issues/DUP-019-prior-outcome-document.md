# DUP-019: The prior-outcome document and invalidated-command chain are built in cognition and again in episodes

- Status: needs-decision
- Severity: medium
- Verdict (finders): REAL
- Themes: persistence
- Wave: not scheduled
- Finder sources: P12 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Not dispatched. Reusing `delta_json` changes the request document and its digests, so it needs a sign-off and a golden refresh. Until then, add the parity test the finder proposes.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P12: The invalidated-command chain and "prior outcome" document are loaded and built twice (cognition and episodes)

- Verdict: REAL
- Shared meaning: for a reconsideration, the executed command with its decision, intent and latest outcome (status, reconciliation status, ordinal, outcome sha, provider result, observed effect).
- Sites:
  - internal/cognition/internal/store/reconsideration.go:80-95 `commands c JOIN intents JOIN decisions JOIN episodes JOIN outcomes WHERE ... policy_status='approved' AND validation_status='accepted' AND c.status='succeeded' AND o.ordinal = MAX` -> `InvalidatedCommand` (domain/correction.go `PriorOutcome()` at :105-120 builds the outcome document with keys status, reconciliation_status, outcome_id, ordinal, outcome_sha256, provider_result, observed_effect), stored as the trigger evaluation `delta_json` (`EvidenceJSON`, :90).
  - internal/episodes/internal/store/reconsideration.go:13-29 re-reads the same chain plus `reconsiderations` (owner cognition) by `reconsideration_id|trigger_id|scheduler_item_id` and builds the outcome document again in internal/episodes/internal/domain/reconsideration.go:100-114 (`outcomeDocument`, same keys plus `command_id`) and `bindOutcomeEvidence`.
  - Related: three of the same joins also carry decision trace context: actions/internal/store/reconciliation.go:20,31, candidates.go:15, approvalledger/internal/store/supersession.go:16-21 (`JOIN intents ... JOIN decisions ... d.traceparent, d.tracestate`) with the same `sql.NullString -> TraceContext` scan boilerplate (15 sites repo-wide).
- How they differ: second copy adds `command_id`, parses JSON into maps, and is bound by a different key; the two documents describe the same outcome at admission time and episode-assembly time and must agree for the replay of reconsideration episodes.
- Risk if left: an outcome column (a reconciliation field) added in one document is missing from the other; reconsideration evidence differs between the stored `delta_json` and the request the worker sees.
- Proposed canonical owner: `internal/cognition` (owns `reconsiderations` and the first load); episodes (layer 21) already can import cognition? Check: `internal/episodes` allowed imports do not include cognition and cognition store imports episodeledger; to avoid a new edge, derive the outcome document once in cognition at detection time (it already writes `delta_json`, which episodes also reads as `Evaluation.DeltaJSON`) and have episodes reuse `delta_json["prior_outcome"]` instead of re-reading the chain; or move the document builder to a shared lower module.
- Proposed fix: episodes assembles from `delta_json` (already loaded by assembler.go:28) and drops its 6-table join and `outcomeDocument`; only the `command_json`/prior decision fields not already in `delta_json` need adding to `EvidenceJSON`. This changes the request document, so it is a digest-affecting design decision; needs a golden refresh and sign-off.
- Behaviour to preserve: request JSON bytes and digests for existing reconsideration fixtures, `reconsiderations` uniqueness `(situation_id, superseded_version, invalidated_command_id)`.
- Verification: cognition correction tests, episodes assembler reconsideration tests, replay goldens containing reconsider episodes; add a test that both builders produce equal `prior_outcome` for the same row until one is removed.

## Outcome

Not started.
