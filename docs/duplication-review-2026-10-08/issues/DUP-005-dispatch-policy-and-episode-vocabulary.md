# DUP-005: Dispatch policy (shadow/active), kind and lane vocabularies; unset must mean shadow everywhere

- Status: open
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

Not started.
