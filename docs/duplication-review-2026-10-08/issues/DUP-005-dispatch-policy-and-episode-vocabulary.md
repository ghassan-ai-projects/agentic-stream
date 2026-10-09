# DUP-005: Dispatch policy (shadow/active), kind and lane vocabularies; unset must mean shadow everywhere

- Status: fixed
- Severity: high (fail-open polarity)
- Verdict (finders): DIVERGED, REAL
- Themes: business rules, contracts and shapes, persistence
- Wave: 2
- Finder sources: P5, R6, S5 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Resolve the owner conflict: `spec` (layer 11) owns the enum and default and sits below `episodeledger` (13), so `spec` exports the dispatch constants and `EffectiveDispatchPolicy`; ledger, episodes, remote and replay import from it. `runner_decision.go` must treat anything other than an explicit active as shadow (fail-safe). Kind and lane constants go to `episodeledger`; remote stops accepting aliases only after confirming no worker sends them. Compiled spec digests must not change.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P5: "Unset dispatch policy means shadow" is implemented in five places, and one consumer treats unset as active

- Verdict: DIVERGED
- Shared meaning: `episodes.dispatch_policy` in {active, shadow}; when the spec leaves it undeclared the default is shadow (nothing enters action governance).
- Sites:
  - migrations/024_episode_modes.sql:7 `DEFAULT 'shadow' CHECK (dispatch_policy IN ('active','shadow'))`.
  - internal/spec/internal/domain/normalize.go:49-50 sets `"shadow"` when empty (rides the compiled digest).
  - internal/episodeledger/internal/domain/admission.go:10,25-29 `DispatchShadow` (private) and `EffectiveDispatchPolicy()` empty -> shadow (a second default, masks an invalid empty value that the CHECK would reject).
  - internal/executor/remote/internal/domain/request_fields.go:21-25 `policy == "active"` else shadow (empty/unknown = shadow, fail-safe).
  - internal/episodes/internal/app/runner_decision.go:75 `if claim.req.DispatchPolicy == "shadow"` else branch calls `persistValidatedIntents` (empty/unknown = ACTIVE governance, fail-open).
  - internal/replay/internal/transport/shadow_worker.go:68 literal `"shadow"`.
- How they differ: three opposite handling of an empty/unknown value (shadow in ledger and remote adapter, active in the episodes runner). Not reachable today because rows always carry a valid non-empty value, so currently latent; the polarity is nevertheless inconsistent and the unsafe one is on the path that writes intents.
- Risk if left: a future spelling/third mode or a code path that builds a `Request` without the field would write intents (governed action path) instead of shadow-scoring.
- Proposed canonical owner: `internal/spec` (layer 9-11) owns the enum and default (it is already imported by episodes, remote domain, replay app/store). Export `spec.DispatchShadow`, `spec.DispatchActive` and `spec.EffectiveDispatchPolicy`. episodeledger (layer 13 -> spec edge is downward, new allowedImports edge `internal/episodeledger/internal/domain -> internal/spec`; episodeledger domain currently has zero deps, so alternatively keep the constants in episodeledger and have spec/normalize take them: spec (11) cannot import episodeledger (13), so spec must be the owner).
- Proposed fix: replace the literals with the spec constants; change runner_decision.go:75 to `!= spec.DispatchActive` (fail-safe: only an explicit "active" reaches governance); drop `EffectiveDispatchPolicy` fallback and let the CHECK reject empties (or keep one default in spec only).
- Behaviour to preserve: compiled spec digest (default "shadow" is part of it), stored value `shadow`/`active`, shadow recording path.
- Verification: existing shadow-mode tests in episodes (episodes/internal/store/store_test.go, runner tests); add test that `DispatchPolicy: ""` and `"bogus"` never persist intents.

### Finder report R6: Dispatch policy ("shadow"/"active") vocabulary and defaulting

- Verdict: REAL
- Shared meaning: an episode runs in `shadow` (score only, never enters governance) or `active`; absent means shadow.
- Sites:
  - internal/spec/internal/domain/normalize.go:49-50 - default `"shadow"` literal; schema enum internal/spec/internal/domain/schema.json:618 (`["active","shadow"]`).
  - internal/episodeledger/internal/domain/admission.go:10,23-29 - `DispatchShadow = "shadow"` and `EffectiveDispatchPolicy` (empty -> shadow), but the const is not re-exported by the episodeledger facade (grep: only used inside the domain).
  - internal/episodes/internal/app/runner_decision.go:75 - `claim.req.DispatchPolicy == "shadow"` literal; anything else (including a stray empty) is treated as active.
  - internal/executor/remote/internal/domain/request_fields.go:21-26 - `policy == "active"` -> ACTIVE else SHADOW (the opposite default direction).
  - internal/replay/internal/transport/shadow_worker.go:68 - `DispatchPolicy: "shadow"` literal.
- How they differ / already diverged: the safe direction is inconsistent: the runner (the gate that decides whether intents are persisted) defaults empty to ACTIVE, while episodeledger and the remote wire map default empty to SHADOW. Today the durable column is always normalized, so this is latent; a future path that builds an `episodes.Request` without that normalization would persist intents (governance) by default.
- Risk if left: shadow-first (P8) is a release-blocking invariant; the "which side is the default" decision should exist once.
- Proposed canonical owner: `internal/episodeledger` (owns `DispatchShadow`, normalization at admission; episodes, replay already import it or episodes; replay/transport needs an edge via episodes facade).
- Proposed fix: export `episodeledger.DispatchShadow/DispatchActive` and a `DispatchPolicy.IsShadow()` where empty is shadow; runner and remote enum map and replay worker use them; spec keeps the schema enum but its default literal is the one place that can stay (rank 11 < 13).
- Behaviour to preserve: durable `dispatch_policy` values, spec digest (default is in compiled digest), protobuf enum mapping.
- Verification: episodeledger rules_test.go:145, episodes runner shadow tests, experiment_shadow_test.go. New: runner test with `DispatchPolicy == ""` expecting shadow handling.

### Finder report S5: Episode/scheduler vocabulary (kind, lane, dispatch policy, status) as string literals in 9 modules with constants in only two

- Verdict: DIVERGED
- Shared meaning: scheduler item `kind` in {standard, reconsider}, `lane` in {fast, deep}, `status` pending..., dispatch policy in {active, shadow} (also a SQL CHECK in migrations/024_episode_modes.sql:6-7).
- Sites:
  - internal/episodeledger/internal/domain/admission.go:9-12 - `DispatchShadow`, `KindReconsider` (the owner's constants); scheduler.go:10-19 documents status/lane/kind only in comments
  - internal/runtime/internal/domain/admission.go:15-16 - second constant `ReconsiderKind = "reconsider"`
  - internal/cognition/internal/domain/correction.go:126,134,136 and timing.go:90 - literals `"reconsider"`, `"standard"`, `"deep"`, `"pending"` when building `episodeledger.SchedulerItem`
  - internal/episodes/internal/app/assembler_inputs.go:68 - `item.Kind == "reconsider"`; internal/episodes/internal/app/runner_decision.go:75 - `DispatchPolicy == "shadow"`
  - internal/decisions/internal/domain/intent.go:78 - `input.Kind != "reconsider"`; internal/replay/internal/domain/shadow_rules.go:103 - `Kind: "standard"`; internal/replay/internal/transport/shadow_worker.go:68 - `DispatchPolicy: "shadow"`
  - internal/spec/internal/domain/references.go:27 - lane `fast`/`deep`; normalize.go:50 - default `"shadow"`
  - internal/executor/remote/internal/domain/request_fields.go:21-26 (policy: anything but `"active"` is shadow), :90-99 (`episodeKind` maps `standard|diagnose|diagnosis` and `reconsider|reconsideration` case-insensitively), :101-112 (`episodeLane` also accepts `batch`)
- How they differ / already diverged: (1) remote's `episodeKind`/`episodeLane` accept aliases and `batch` that no other module produces or checks; episodes/decisions compare exact lowercase. (2) Empty dispatch policy means shadow in `Admission.EffectiveDispatchPolicy` (admission.go:25-30), remote (`!= "active"`) and the SQL default, but `runner_decision.go:75` treats only the literal `"shadow"` as shadow, so an empty value would govern as active; the DB CHECK currently prevents it, which hides the inconsistency. (3) The same `"reconsider"` is a constant in episodeledger and runtime but a literal in cognition, episodes and decisions. Probably accidental.
- Risk if left: a new kind or policy value (batch lane, new dispatch mode) needs ~10 edits; the empty-vs-shadow asymmetry becomes a fail-open the day a code path builds a `Request` without the DB column.
- Proposed canonical owner: `internal/episodeledger` (layer 13, owns the scheduler/episode lifecycle tables and already exports `KindReconsider`, `DispatchShadow`); cognition, episodes and runtime already import it. decisions (layer 17) and spec (layer 11) cannot import it; give spec its own enum validation (it owns the SituationSpec lane field) and have decisions receive `Kind` as already-parsed input.
- Proposed fix: add `KindStandard`, `LaneFast`, `LaneDeep`, `ItemPending`, `DispatchActive` plus `Admission.EffectiveDispatchPolicy` reuse as `episodeledger.EffectiveDispatch(policy string)`; replace literals in cognition/episodes/replay/runtime; remote maps through the same constants and rejects unknown kinds instead of accepting aliases (confirm no worker sends aliases first); delete `runtime.ReconsiderKind`.
- Behaviour to preserve: durable strings (migrations CHECK, golden ledgers), remote error text `unsupported episode kind %q`, shadow-mode behaviour (invariant 7).
- Verification: existing remote request tests, episodes shadow tests, cognition correction tests. New: a test that every value accepted by `remote.episodeKind` is a declared constant, and a test asserting empty dispatch policy is treated as shadow by both remote and `governDecision`.

## Outcome

Status: fixed. Commit: f039750.

Verified (all re-read):
- Confirmed: `runner_decision.go` treated only the literal `"shadow"` as shadow, so an empty or unknown policy governed as active (fail-open); remote and ledger defaulted to shadow. A new runner test seeded a shadow row, bypassed the CHECK with `PRAGMA ignore_check_constraints`, and shows the old polarity would have written intents.
- Confirmed: literals `"reconsider"`/`"standard"`/`"deep"` in cognition, episodes, decisions, replay and a second `runtime.ReconsiderKind`; remote `episodeKind`/`episodeLane` accepted `diagnose`, `diagnosis`, `reconsideration`, `batch` and mixed case.
- Partly right: the proposed owner. `episodeledger/internal/domain` is layer 0 and cannot import `spec` (layer 11), and `spec` cannot import `episodeledger` (13). So the dispatch policy and the lane vocabulary live in `spec` (the authoring side, which also owns the compiled default that rides the digest); kinds live in `episodeledger`.
- Aliases and `batch`: the host builds the request JSON `kind` from `SchedulerItem.Kind` (`standard` or `reconsider`) and the lane from the spec-validated trigger lane (`fast`/`deep`); a worker never sends them. Only four test fixtures used `"diagnose"`.
- Not done: the scheduler item status literal `"pending"` (cognition correction.go, timing.go). It belongs with the lifecycle/status sets of DUP-004; left as is.

Changed:
- `internal/spec/internal/domain/vocabulary.go` (new): `DispatchShadow`, `DispatchActive`, `LaneFast`, `LaneDeep`, `EffectiveDispatchPolicy`; `normalize.go` and `references.go` use them; `internal/spec/spec.go` re-exports them. Compiled digests unchanged (default is still `shadow`).
- `internal/episodeledger/internal/domain/admission.go`: `KindStandard` and `KindReconsider`; `DispatchShadow` and `Admission.EffectiveDispatchPolicy` deleted (a second default that masked invalid empties); `internal/episodeledger/kinds.go` (new) exports the kinds; `internal/app/admission.go` stores the declared policy.
- `internal/episodes/internal/domain/admission.go`: `AdmittedEpisode` is now the single place that applies `spec.EffectiveDispatchPolicy` before the ledger. `runner_decision.go`: `!= spec.DispatchActive` is shadow (fail-safe). `assembler_inputs.go` and `decision_input.go` use `episodeledger.KindReconsider`.
- `internal/decisions`: `Input.Kind string` replaced by `Input.Reconsider bool` (decisions, layer 13, cannot import the ledger or spec; it now receives the parsed fact). `intent.go`, `validator.go`, `facade_test.go`, `validator_test.go`, and `replay/internal/domain/shadow_rules.go` (dropped the hardcoded `Kind: "standard"`) follow.
- `internal/cognition/internal/domain/correction.go`, `timing.go`: kind and lane constants.
- `internal/runtime/internal/domain/admission.go`: `ReconsiderKind` deleted; `internal/runtime/internal/app/admission_skip.go` uses `episodeledger.KindReconsider`.
- `internal/executor/remote/internal/domain/request_fields.go`: policy mapping uses `spec.DispatchActive`; `episodeKind` accepts exactly `standard` and `reconsider`, `episodeLane` exactly `fast` and `deep` (aliases, `batch` and case folding removed; error text `unsupported episode kind %q` unchanged).
- `internal/replay/internal/transport/shadow_worker.go`: `spec.DispatchShadow`; `architecture_test.go` gained the allowedImports edge `internal/replay/internal/transport -> internal/spec`.
- `internal/episodeledger/UBIQUITOUS_LANGUAGE.md`: dispatch policy row now names the spec as owner.

Deliberate behaviour changes: (1) the ledger no longer defaults an empty policy; an empty `Admission.DispatchPolicy` is refused by the table CHECK (fail closed). Production reaches the ledger only through `AdmittedEpisode`, which defaults it. (2) the remote worker request builder rejects the old aliases, `batch` and upper case. Fixture changes that follow: `episodeledger/ownership_test.go` (renamed to `TestAdmissionStoresDeclaredPolicyAndRejectsConflictingLiveEpisode`, sets `DispatchPolicy: "shadow"`), `episodeledger/internal/app/app_test.go` (fixture sets the policy), `episodeledger/internal/domain/rules_test.go` (fallback assertion removed), `"kind":"diagnose"` fixtures changed to `"standard"` in `executor/remote/internal/app/executor_test.go`, `executor/remote/internal/domain/request_test.go` and `testsupport/executorconformance/conformance.go`, and the `diagnose` row of `TestEpisodeKind` now expects rejection.

Tests that pin the rule:
- `TestOnlyAnExplicitActivePolicyEntersGovernance` (episodes/internal/app; `""`, `bogus`, `ACTIVE` never persist intents and are shadow-scored).
- `TestUnsetDispatchPolicyIsShadow` and `TestSchemaEnumsMatchDeclaredVocabulary` (spec/internal/domain; schema enums equal the constants, compiled default is shadow).
- `TestAdmittedEpisodeDeclaresShadowWhenRequestHasNoPolicy` (episodes/internal/domain).
- `TestAdmissionRefusesAnUndeclaredDispatchPolicy` (episodeledger; no hidden default in the ledger).
- `TestDispatchPolicyEnumTreatsAnythingButActiveAsShadow`, `TestEpisodeLaneAcceptsOnlyDeclaredLanes`, `TestEpisodeKind` (executor/remote/internal/domain).

Review fix R2-F4: the implicit `R1` risk ceiling is gone from `internal/episodes/internal/domain/assembly.go` (`effectiveRiskCeiling`) and `internal/replay/internal/domain/shadow_rules.go` (`riskCeiling == "" -> "R1"`). Compiled specs are always defaulted to `R1` by the spec compiler (`defaultCognition`), so production is unaffected. A `CompiledSpec` built by hand without the compiler now carries `risk_ceiling: ""`, which `DecisionInput` refuses ("request has no explicit risk ceiling") and which makes shadow validation reject every intent (rank 0). This fails closed. The single place that defaults the ceiling is the spec compiler; test fixtures that build a `CompiledSpec` directly must set `RiskCeiling`.
