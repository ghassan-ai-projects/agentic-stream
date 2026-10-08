# DUP-009: The episode request_json document is written as a map and decoded by seven readers; the budget is declared four times

- Status: open
- Severity: high
- Verdict (finders): DIVERGED, REAL
- Themes: business rules, contracts and shapes
- Wave: 3
- Finder sources: S4, R10 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `episodes` exports a typed request document and budget with metric constants. Canonical bytes and key names must be identical (admission keys and goldens depend on them). Exhaustion reasons differ between native (`budget_exhausted:<metric>`) and remote (`budget_exhausted`); pin one vocabulary with the tests that assert it and record the choice in the outcome.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S4: The episode `request_json` document is written as a map in `episodes` and decoded by six independent readers; the budget block is declared four times

- Verdict: REAL
- Shared meaning: `episodes.requestJSON` is a single durable document (kind, trigger, snapshot, tools, allowed_intent_types, risk_ceiling, executor{...}, budget{...}, reconsideration, cancellation/supersession keys); every consumer needs the same field names and types.
- Sites:
  - internal/episodes/internal/domain/assembly.go:55-92 - writer: `requestIdentity`, `bindRequestCapabilities`, `bindRequestEvidence` (map literals with 20+ string keys)
  - internal/episodes/internal/domain/assembly_executor.go:28-34,48-58,93-95,143-156 - writer of `executor{...}` and `budgetMap` (9 snake_case budget keys, mapped from `spec.Budget` camelCase at internal/spec/internal/domain/spec.go:191-201)
  - internal/episodes/internal/domain/decision_input.go:13-23 - reader 1: `decisionRequestAuthority` (allowed_intent_types, risk_ceiling, kind, executor.intent_catalog[_sha256])
  - internal/episodes/internal/domain/budget.go:11-42 and admission.go:57-79 - readers 2/3: anonymous structs for `budget.wall_time` and `budget.cost_microunits`, plus `reboundRequestJSON` (admission.go:58) re-edits the map by key
  - internal/executor/native/internal/domain/request.go:13-35 - reader 4: `RequestPayload` incl. the 8-field budget struct
  - internal/executor/remote/internal/domain/request.go:18-60 - reader 5: `workerRequestPayload` incl. the same 8-field budget struct, `executor` block with 10 fields, `reconsideration`
  - internal/executor/fixture/executor.go:42-58 - reader 6: untyped `payload["snapshot"]["phase"]`, `payload["trigger"]["trigger_name"]`
  - internal/replay/internal/transport/shadow_worker.go:77-85 - reader 7: `promptProvenance` re-declares `executor{prompt...}`
  - internal/testsupport/executorconformance/conformance.go:46 - hand-written JSON string with the same keys
- How they differ: the field sets are subsets of one shape and are consistent today (`total_tool_result_bytes`, `cost_microunits` spellings match), but the native and remote budget structs are byte-identical copies, the writer and readers share no type, and nothing fails a build if the writer renames a key (readers silently get zero values; `RequestBudget` then fails only at run time with "finite episode budget requires wall_time or model_calls").
- Risk if left: adding or renaming one budget dimension or executor field is a 6-site change with a silent zero-value failure mode; the budget block is also the cost-ceiling input, so a mismatch is a control-plane fault, not a cosmetic one.
- Proposed canonical owner: `internal/episodes` (it writes the document and already is imported by native, remote, fixture and replay; episodes domain stays below executors per `forbiddenImports`/A8). Export a typed `episodes.RequestDocument` (and `RequestBudget`) from `internal/episodes/internal/domain` used by the writer (marshal the struct instead of building a map) and decoded by `episodes.DecodeRequestDocument(raw)`.
- Proposed fix: define the struct once; the assembler fills and canonicalises it; native/remote/episodes readers embed or project from it; delete the two copied budget structs and the two anonymous budget readers. Keep the map form only inside `reboundRequestJSON` or convert it to set struct fields.
- Behaviour to preserve: the canonical bytes of `request_json` are digest-adjacent (`admission_key`, stored `request_json`, replay goldens), so field set, key names and omit-empty behaviour must be identical; `reconsideration` and `watch_confidence_floor` must still be omitted/null exactly as today.
- Verification: assembly golden tests in episodes, `TestAllBuiltinsLoadFromData`, executor conformance suite, remote request tests. New: a round-trip test (assemble -> canonical JSON -> struct) asserting byte equality with the current golden request JSON, and a reflection test that every `json` tag of the struct appears in the golden.

### Finder report R10: Episode budget: metric set, "zero means unlimited" and exhaustion reasons implemented per executor

- Verdict: DIVERGED
- Shared meaning: the nine budget metrics (wall_time, model_calls, input/output tokens, tool_calls, tool_result_bytes, total_tool_result_bytes, provider_retries, cost_microunits), where 0 means unlimited, and the exhaustion outcome.
- Sites:
  - internal/spec/internal/domain/spec.go:192ff and schema.json:452-465 - authoring shape (all required).
  - internal/episodes/internal/domain/assembly_executor.go:140-153 - `budgetMap()` re-spells the nine JSON keys into the request document.
  - internal/episodes/internal/domain/budget.go:11-26 and request_budget.go - `ParseWallTimeBudget` (>0, positive).
  - internal/executor/native/internal/domain/request.go:23-33 (anonymous Budget struct with the nine json tags), :35-52 `RequestBudget` (adds the rule "finite budget requires wall_time or model_calls").
  - internal/executor/remote/internal/domain/request.go:42ff and :232-239 `wireBudget` (the nine fields again) + internal/worker/internal/domain/budget.go:12-19 `ValidateBudget` (wall_time must be valid and positive).
  - Enforcement: internal/executor/native/internal/domain/budget.go:42-53 (`budget_exhausted:input_tokens` etc. as bare error strings) and native/app/loop.go:62,108 / loop_tools.go:19,76 (`Failed(req, "budget_exhausted:model_calls"...)`) versus internal/executor/remote/internal/domain/budget.go:44,98-133 (typed `episodes.BudgetExceededError{Metric}`), mapped by internal/episodes/internal/domain/failure.go:24-28 to the single reason `budget_exhausted`.
- How they differ / already diverged: the same exhaustion is recorded as reason `budget_exhausted:model_calls` on the native path (metric in the reason) and `budget_exhausted` on the remote path (metric dropped); native accepts a budget with `model_calls` but no `wall_time`, remote/worker refuse it; metric names are spelled independently ("cost_microunits" key vs `Metric:` strings vs cost_microunits error).
- Risk if left: adding a metric (say max_output_bytes) is a 6-place change; dashboards/audits that group attempts by reason see two vocabularies for the same event.
- Proposed canonical owner: `internal/episodes` (rank 24; already exports `Request.WallTimeBudget`, `BudgetExceededError`; native and remote already import it). Add `episodes.Budget` (decoded once from RequestJSON via the existing `decodeBudget`) with `Metric` constants, and have both executors read it.
- Proposed fix: single decoded `episodes.Budget` struct + metric name constants; native returns `BudgetExceededError{Metric}` (or maps to the same reason text); drop the duplicated anonymous structs; decide the finite-budget rule once (spec already requires wallTime, so the native "or model_calls" relaxation is dead).
- Behaviour to preserve: request document key names (digest-bound request JSON), `budget_exhausted` reason for the remote path, whichever native reason tests pin (native loop tests assert the `:metric` suffix, so either keep the suffix on both paths or change both with the tests).
- Verification: native loop/budget tests, remote budget tests, episodes failure tests, experiment recorded tests. New: a table test running both executors' limit logic over the same budget and asserting identical reasons.

## Outcome

Not started.
