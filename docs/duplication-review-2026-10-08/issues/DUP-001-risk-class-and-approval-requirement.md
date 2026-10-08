# DUP-001: Risk class order, routes and "needs approval" are decided in eight places

- Status: fixed
- Severity: high (probable bug)
- Verdict (finders): DIVERGED
- Themes: business rules, contracts and shapes, persistence
- Wave: 1
- Finder sources: R1, R2, S2, P10 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `contractsv1` for the pure `RiskClass` (order, validity, default ceiling) and for a pure `ApprovalRequired(risk, requiresApproval)` that policy, actions and shadow scoring all call; `policy` keeps the route reasons. Fix the gaps the finders found, after confirming each by reading: shadow scoring ignores `RequiresApproval`; actions `CheckApproval` only checks the approval record for R2. Shadow scores for R0/R1 with `requires_approval` change on purpose (shadow must report what live would do); say so in the outcome. The policy digest document must be derived from the same table so a parity test can pin it.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R1: "Which intents need a human approval" is decided four ways

- Verdict: DIVERGED
- Shared meaning: whether an intent must go through approval before it may run, given its risk class, the catalog `requires_approval` flag, and the live policy.
- Sites:
  - internal/policy/internal/domain/routing.go:11-24 - `RiskRoute`: R0/R1 -> "approval" only when `row.RequiresApproval != 0`, else "automatic"; R2 -> "approval"; R3/R4 -> denied; else `unknown_risk_class`.
  - internal/policy/internal/domain/definition.go:49-53 - `CanonicalDocumentForVersion` declares the same matrix as data (`"R0":"automatic","R1":"automatic","R2":"approval",...`) and the `incomplete_source_health` table (`R2,R3,R4 -> denied`). It is digested into policy evidence (`DigestForVersion`) but is NOT what `RiskRoute` reads.
  - internal/policy/internal/domain/rules.go:51 - `SourceHealthIncomplete` re-types the incomplete-source-health table as `RiskClass == "R2" || "R3" || "R4"`.
  - internal/policy/internal/domain/principals.go:47,149 - `approvableRisks = {"R0","R1","R2"}`: which risks an approver may be granted.
  - internal/actions/internal/domain/authorization.go:74-85 - `CheckApproval` at dispatch: `if r.Intent.Risk != "R2" { return nil }`, so the approval-record-present and approval-not-expired checks run only for R2. `IntentRow` (line 12) carries no requires-approval flag.
  - internal/episodes/internal/domain/shadow.go:41-50 - `ScoreShadowDecision`: R0/R1 -> would_approve, R2 -> would_require_approval, else would_deny. It never looks at `RequiresApproval`, which `decisions.Intent` carries (internal/decisions/internal/domain/intent.go:139).
- How they differ / already diverged:
  - `spec/internal/domain/normalize.go:55-58` defaults every intent's `Policy` to "approval", so `requires_approval=true` is the DEFAULT for R0/R1 intents. For those intents `RiskRoute` demands an approval, but `CheckApproval` (actions) skips the approval-present / not-expired re-check at dispatch because the risk is not R2. Unit test authorization_test.go:76 asserts R1 passes `CheckApproval` with no approval record. `RequireApprovedIntent` still demands `policy_status == "approved"`, so this is a weakened second gate, not an open door. It looks like a latent gap (probable bug), not an ADR-backed choice.
  - Shadow scoring reports "would_approve" for an R1 intent that the live policy would route to approval (the doc comment on shadow.go:12 says it is "what EvaluateIntent WOULD have decided").
  - The digested policy document (definition.go) can be edited without changing behaviour, and the reverse.
- Risk if left: changing the approval policy (say R1 always needs approval, or a new risk tier) requires editing 5 places; the digest-bound policy document will silently disagree with the code that enforces it; shadow KPIs misreport policy outcomes.
- Proposed canonical owner: policy domain owns the matrix: `RiskRoute`, `SourceHealthIncomplete` and `CanonicalDocumentForVersion` should all read one `riskPolicy` table. Episodes may not import policy (forbidden table, architecture_test.go:51), so the shadow scorer needs the same table through a lower pure module; the natural home is next to `RiskClass` in `contractsv1` (see C2), exposing `ApprovalRequired(risk, requiresApproval) bool` that policy `RiskRoute` also calls. Actions (25) re-reads the persisted fact instead of re-deriving it.
- Proposed fix: add `RequiresApproval bool` to `actions.IntentRow` (store/authorization.go already selects from `intents i`), make `CheckApproval` apply to every intent where `RequiresApproval || risk==R2`; derive `CanonicalDocumentForVersion().risk_policy` and `incomplete_source_health` from the same table `RiskRoute`/`SourceHealthIncomplete` use; make `ScoreShadowDecision` honour `RequiresApproval`.
- Behaviour to preserve: policy digest (`DigestForVersion`) bytes; reason strings `risk_policy_denied`, `unknown_risk_class`, `source_health_incomplete`; actions error text "approved intent has no approved approval record" / "approval is expired"; shadow reasons `would_approve_R1`, `would_require_approval_r2`, `would_deny_R3`.
- Verification: policy rules tests, actions/internal/domain/authorization_test.go (line 76 must change to expect failure for an approval-required R1 without approval row), episodes shadow tests, TestAquaculture/e2e experiment tests. New: a table test asserting the policy document, `RiskRoute`, `CheckApproval` and shadow scoring agree for every (risk, requires_approval) pair.

### Finder report R2: Risk class vocabulary, ordering and defaults

- Verdict: DIVERGED
- Shared meaning: the closed set R0..R4, its total order, and the default episode ceiling R1.
- Sites:
  - internal/decisions/internal/domain/validator.go:130-145 - `riskRank`: R0=1,R1=2,R2=3,R3=4,R4=5, unknown=0.
  - internal/episodes/internal/domain/shadow.go:88-102 - `riskRank`: R0 and unknown both 0, R1=1,...,R4=4 (second ranking, different numbering).
  - internal/decisions/internal/domain/catalog.go:66 - validity test `riskRank(risk) == 0`.
  - internal/decisions/internal/domain/intent.go:96 - ceiling check `riskRank(risk) > riskRank(input.RiskCeiling)`.
  - internal/episodes/internal/domain/intent_catalog.go:84 - validity as an explicit five-way `!= "R0" && ... && != "R4"` chain over the spec intent.
  - internal/executor/remote/internal/domain/request_fields.go:114-120 - `riskClass()`: upper-cases and trims, accepts `r1` and ` R1 `, maps via byte arithmetic to the proto enum.
  - internal/spec/internal/domain/normalize.go:43-44 - default ceiling `"R1"`.
  - internal/episodes/internal/domain/assembly_executor.go:135-139 - `effectiveRiskCeiling`: second default `"R1"`.
  - internal/replay/internal/domain/shadow_rules.go:37-40 - third default `"R1"`.
  - internal/episodes/internal/domain/decision_input.go:37-38 - the opposite rule: empty ceiling is an error ("request has no explicit risk ceiling").
  - internal/policy/internal/domain/rules.go:51, routing.go:13-20, principals.go:47 and actions/internal/domain/authorization.go:75 - R-sets spelled as string literals (see C1).
  - internal/executor/fixture/executor.go:83 - fixture intent hard-codes "R1".
  - Schema enums (not Go, listed for completeness): internal/spec/internal/domain/schema.json:615,651; internal/contractsv1/internal/domain/schemas/v1/intent-v1.json:15; internal/notify/internal/domain/contracts/notification-contract-v1.json:52.
- How they differ / already diverged: two `riskRank` functions with different numbering; the episodes one cannot tell R0 from an invalid value (harmless today because validated intents are always valid, but `highestRiskIntent` on an all-R0 decision picks `Intents[0]` by accident of `>`). The remote executor accepts lowercase/padded risk strings the other four sites reject. The R1 default exists in three places while the consuming validator refuses an empty ceiling, so two of the three defaults are dead code and one of them (spec) is the only live one.
- Risk if left: adding a tier or changing the default ceiling needs ~10 edits; a ranking disagreement between decisions (ceiling check) and episodes (shadow highest-risk pick) would score a shadow decision against a different intent than the validator reasoned about.
- Proposed canonical owner: `internal/contractsv1` (it owns the `risk_class` enum in intent-v1.json; decisions/domain, policy/domain, actions/domain, episodes/domain and replay/domain all already import it, so no new edge for them; remote executor domain needs one added edge). `spec` (rank 11) is below contractsv1 (12) so it keeps its own schema enum and the one live default.
- Proposed fix: add `contractsv1.RiskClass` (string type) with constants R0..R4, `Valid()`, `Rank()`, `AtMost(ceiling)`; delete both `riskRank`s, the five-way chain in intent_catalog.go and the dead `"R1"` fallbacks in assembly_executor.go and shadow_rules.go (spec normalization already guarantees non-empty); make remote `riskClass()` use `Valid()` (decide explicitly whether to keep case-insensitivity). Keep `spec` default as the single default.
- Behaviour to preserve: reject reasons `risk_label_mismatch`, `risk_ceiling_exceeded`; error text "intent %q has invalid declared risk %q" (decisions and episodes use the same text, keep it); spec digest (the default is baked into the compiled digest).
- Verification: decisions validator tests, episodes intent_catalog and shadow tests, replay shadow tests, remote request tests. New: one table test in contractsv1 pinning order R0<R1<R2<R3<R4 and rejection of "", "r1", "R5".

### Finder report S2: Risk-class table encoded in 8 places; shadow scoring ignores `RequiresApproval` that the live route honours

- Verdict: DIVERGED
- Shared meaning: risk classes R0..R4 have an order, a default ceiling (R1) and a live route (R0/R1 automatic or approval if the catalog entry requires it, R2 approval, R3/R4 denied).
- Sites:
  - internal/policy/internal/domain/routing.go:11-24 - `RiskRoute`: R0/R1 -> approval when `RequiresApproval != 0` else automatic; R2 approval; R3/R4 denied; else `unknown_risk_class`
  - internal/policy/internal/domain/definition.go:48-53 - the SAME route table again as a map in `CanonicalDocumentForVersion` (it is hashed into the policy digest)
  - internal/episodes/internal/domain/shadow.go:41-52 - `ScoreShadowDecision`: R0/R1 -> would_approve, R2 -> would_require_approval, else would_deny (claims to be "the would-be policy outcome ... under the live policy")
  - internal/episodes/internal/domain/shadow.go:88-102 - `riskRank` R1=1..R4=4, R0 and unknown both 0
  - internal/decisions/internal/domain/validator.go:130-145 - `riskRank` R0=1..R4=5, unknown 0
  - internal/episodes/internal/domain/intent_catalog.go:84 - five-way `!= "R0" && ... "R4"` validity check
  - internal/executor/remote/internal/domain/request_fields.go:114-119 - `riskClass` accepts `R0`..`R4` by byte arithmetic (ceiling to proto enum)
  - internal/policy/internal/domain/rules.go:51 - consequential = R2||R3||R4; internal/policy/internal/domain/principals.go:47 - `approvableRisks` R0..R2; internal/actions/internal/domain/authorization.go:75 - `Risk != "R2"` approval gate
  - Default ceiling "R1": internal/spec/internal/domain/normalize.go:44, internal/episodes/internal/domain/assembly_executor.go:136-139 (`effectiveRiskCeiling`), internal/replay/internal/domain/shadow_rules.go:38-40
- How they differ / already diverged: `ScoreShadowDecision` reads only `highest.RiskClass`; `decisions.Intent.RequiresApproval` (validator.go:49) is available on the same value but unused. A R0/R1 intent whose catalog entry sets `requires_approval` is routed to approval live (routing.go:14) yet scored `would_approve` in shadow, so shadow-first calibration evidence (the thing that unlocks automatic consequential intents) overstates auto-approval. The two `riskRank` functions use different numbering (R0 is 1 in decisions, 0 in episodes; harmless today only because the values are never mixed). Unknown risk: policy denies with `unknown_risk_class`, shadow scoring falls through to `would_deny_<anything>`. Looks like an unintended drift, not a design choice (no ADR; grep of DECISIONS.md finds nothing for shadow scoring).
- Risk if left: adding a risk class or changing a route (for example R2 auto with approver) needs edits in policy route, policy digest document, shadow scoring, validator rank, remote enum and catalog check; the policy digest document and `RiskRoute` can disagree and nothing tests it.
- Proposed canonical owner: `internal/contractsv1` (layer 12; already owns the intent schema and the `R0..R4` enum, imported by decisions/policy/episodes/actions). Add `contractsv1.RiskRank(string) (int, bool)` and a `RiskClasses` list; keep the route in `policy/internal/domain` and expose a pure `policy.RouteFor(risk, requiresApproval)` that shadow scoring can call.
- Proposed fix: one ordered `RiskClasses` slice in contractsv1; delete both `riskRank`s and the five-way check; derive `CanonicalDocumentForVersion`'s `risk_policy` from `RiskRoute` outputs; make `ScoreShadowDecision` take the route from the policy function (needs episodes -> policy to stay forbidden per `forbiddenImports`, so put the pure route function in contractsv1 or `decisions`, not policy; decisions (layer 17) is already imported by episodes). Default ceiling constant `contractsv1.DefaultRiskCeiling` used by spec, episodes and replay.
- Behaviour to preserve: policy digest (`DigestForVersion`) bytes; `would_*` reason strings recorded in shadow rows; replay shadow goldens. Fixing the RequiresApproval gap changes shadow scores for R0/R1 intents with requires_approval, which is a deliberate, reviewed data change.
- Verification: policy routing tests, `shadow_test.go` in episodes. New: table test that for every (risk, requiresApproval) the shadow score agrees with `RiskRoute`; test pinning `risk_policy` in the policy digest document to `RiskRoute`.

### Finder report P10: Risk class R0..R4: rank, route and validity are re-encoded per module and the two `riskRank` functions disagree

- Verdict: DIVERGED (cross-theme: also belongs to the domain-rules audit; reported here because the set mirrors CHECK constraints)
- Shared meaning: the ordered risk classes and the policy route per class (R0/R1 automatic, R2 approval, R3/R4 denied).
- Sites:
  - CHECK / schema: migrations/001_initial.sql intents.risk_class, migrations/013_governance_interlock.sql:25 approval_authorities.risk_class, internal/contractsv1/internal/domain/schemas/v1/intent-v1.json:15, internal/spec/internal/domain/schema.json:615,651.
  - internal/decisions/internal/domain/validator.go:130-146 `riskRank` R0=1..R4=5, unknown=0.
  - internal/episodes/internal/domain/shadow.go:87-104 `riskRank` R1=1..R4=4, R0 and unknown both 0.
  - internal/episodes/internal/domain/intent_catalog.go:84 five-way `!=` validity check.
  - internal/episodes/internal/domain/shadow.go:44-50 `ScoreShadowDecision` re-derives the policy route (R0/R1 approve, R2 require approval, else deny).
  - internal/policy/internal/domain/routing.go:11-24 `RiskRoute` (the real route, also honours `requires_approval` for R0/R1); policy/internal/domain/definition.go:49-52 the same table as a data document bound into policy evidence; rules.go:51 `consequential` = R2|R3|R4; principals.go:47 `approvableRisks`.
- How they differ: two rank functions with different bases (benign today since only relative order for valid classes is used); shadow scoring ignores `requires_approval` (R0/R1 with requires_approval=1 routes to approval live, "would_approve" in shadow); the route table exists once as code (`RiskRoute`) and once as the digest-bound document (`CanonicalDocumentForVersion`) with no test tying them.
- Risk if left: adding a class or changing a route changes live policy but not shadow comparison or the evidence document; shadow reports misstate what live would do.
- Proposed canonical owner: `internal/decisions` (layer 17) is already imported by episodes and holds `riskRank`; policy domain (layer 13) cannot import it. A small neutral home is `internal/contractsv1` (layer 12; owns the intent schema enum): export `contractsv1.RiskClass` with `Rank()`, `Valid()`. Policy route stays in policy; episodes shadow scoring should call a policy-exported pure function if the layer table allows (episodes store/app are above policy domain? episodes/internal/domain is below policy; if not, keep shadow route but pin it with a parity test).
- Proposed fix: single `RiskClass` type with `Rank()`; delete both `riskRank` copies and the five-way check; add a parity test: for every class and `requires_approval` in {0,1}, `ScoreShadowDecision` route equals `RiskRoute`, and the `risk_policy` map in `CanonicalDocumentForVersion` equals `RiskRoute` output.
- Behaviour to preserve: policy digest (definition.go is digest input), stored class strings, reason codes `risk_policy_denied`, `unknown_risk_class`.
- Verification: policy routing tests, decisions validator tests, episodes shadow tests; the parity test is new.

## Outcome

Status: fixed. Commit: 2179bfb.

Verified (every site re-read):
- Confirmed: `policy` `RiskRoute`, the digest document in `CanonicalDocumentForVersion`, `SourceHealthIncomplete` and `approvableRisks` each spelled the R0..R4 table separately; nothing tied the digested document to the code that enforced it.
- Confirmed (probable bug): actions `CheckApproval` only checked the approval record for `R2`, so an R0/R1 intent with `requires_approval` (the spec default is `policy: approval`) was routed to approval by policy but skipped the approval-present and not-expired re-check at dispatch. `IntentRow` carried no such flag.
- Confirmed (bug): `ScoreShadowDecision` ignored `decisions.Intent.RequiresApproval`, so shadow reported `would_approve` where live routes to approval.
- Confirmed: two `riskRank` functions with different numbering (decisions R0=1, episodes R0=0); a five-way `!=` validity chain in `episodes/intent_catalog.go`; the remote executor accepted `r1` and ` R1 ` by byte arithmetic while every other site rejects them.
- Confirmed: the `"R1"` default ceiling existed in `spec` (live), `episodes` `effectiveRiskCeiling` and `replay` `ShadowRules.ValidateOutput`. The last two were dead in production (spec compile always fills it, and `decision_input.go` refuses an empty ceiling), but several test fixtures hand-built specs without a ceiling and relied on the episodes fallback.
- Not changed, as the issue says: SQL CHECK constraints, `spec` schema.json, `notification-contract-v1.json`, the fixture executor's literal `"R1"` intent risk. `spec` (layer 9/11) cannot import `contractsv1` (12), so its `"R1"` stays the one live default.

What changed:
- New `internal/contractsv1/internal/domain/risk.go` (facade in `contractsv1.go`): `RiskClass` (`Valid`, `Rank`, `AtMost`, `Consequential`, `Approvable`), `RiskClasses()`, `Route` and `RouteFor(risk, requiresApproval)`, plus `RiskPolicyDocument()` and `IncompleteSourceHealthDocument()`. One ordered table drives all of them.
- `policy`: `RiskRoute` calls `RouteFor` and keeps the reasons `risk_policy_denied` and `unknown_risk_class`; `SourceHealthIncomplete` uses `Consequential`; `CanonicalDocumentForVersion` builds `risk_policy` and `incomplete_source_health` from the contractsv1 documents; `approvableRisks` is gone (`Approvable`).
- `decisions`: `riskRank` deleted; validity via `Valid`, ceiling via `AtMost`.
- `episodes`: `riskRank`, the five-way chain and `effectiveRiskCeiling` deleted; `ScoreShadowDecision` scores from `RouteFor`.
- `replay`: dead `"R1"` fallback and the redundant `riskCeiling` parameter of `validateDecision` removed.
- `executor/remote`: `riskClass` uses `Valid` and the proto name table; new allowedImports edge `executor/remote/internal/domain -> contractsv1`.
- `actions`: `IntentRow.RequiresApproval` loaded from `intents.requires_approval`; `CheckApproval` applies wherever `RouteFor == approval` (R2 always, R0/R1 when flagged).
- Test fixtures that relied on the dead default now set `RiskCeiling: "R1"` (as the compiler does): `episodes/internal/app/lifecycle_fixture_test.go` and five spec literals under `internal/runtime`.
- `internal/contractsv1/UBIQUITOUS_LANGUAGE.md`: Risk class and Route rows.

Behaviour changes made on purpose:
1. Shadow scores for R0/R1 intents with `requires_approval` are now `would_require_approval` with reason `would_require_approval_r0|r1` (was `would_approve_R0|R1`). R2 and R3/R4 reasons are byte-identical (`would_require_approval_r2`, `would_deny_R3`).
2. Dispatch now re-checks the approval record (present, not expired) for R0/R1 intents with `requires_approval`. `authorization_test.go` `TestOnlyR2IntentsNeedAnUnexpiredApproval` was replaced because its R1 case has `requires_approval` false and so stays valid; the new tests cover the flagged case.
3. The remote executor rejects `r1` and ` R1 ` (fail closed; the ceiling is always spec-normalised `R0..R4` before it reaches it).
4. A spec without a risk ceiling no longer gets `R1` at assembly or replay: the request is refused ("request has no explicit risk ceiling") or every intent exceeds the ceiling. Compiled specs always carry it.

Preserved: `DigestForVersion("v1")` is byte-identical (pinned at `sha256:473ca136...5fb75e`, captured before the change); reasons `risk_label_mismatch`, `risk_ceiling_exceeded`, `risk_policy_denied`, `unknown_risk_class`, `source_health_incomplete`; error text "intent %q has invalid declared risk %q"; actions text "approved intent has no approved approval record" and "approval is expired".

Deferred / concern: `highestRiskIntent` still picks the highest-ranked intent only. A decision with an R1 automatic intent and an R0 intent that requires approval is scored on the R1 intent, while live would hold the R0 intent for approval. Scoring by the strictest route is a separate behaviour change; left for a decision.

Tests that pin the rule:
- contractsv1: `TestRiskClassesAreOrderedAndClosed` (order R0<..<R4, rejects "", "r1", " R1", "R5"), `TestRiskCeilingComparesByRank`, `TestRouteForEveryRiskAndApprovalFlag`, `TestPolicyDocumentsDeriveFromTheRiskTable`, `TestSchemaRiskEnumMatchesTheRiskTable` (intent-v1.json enum equals the table).
- policy: `TestPolicyDocumentAndRoutesAgreeWithTheRiskTable` (RiskRoute, digest document and SourceHealthIncomplete agree for every class and flag), `TestPolicyDigestIsPinned`, `TestOnlyApprovableRisksMayBeGranted`.
- actions: `TestApprovalIsCheckedExactlyWhereThePolicyRoutesToApproval`, `TestApprovalRequiredIntentsNeedAnUnexpiredApproval`, store `TestAuthorizationRecordsCarryTheCatalogApprovalRequirement`.
- episodes: `TestShadowScoreAgreesWithTheLiveRouteForEveryRisk`, `TestShadowScoreHonoursRequiresApprovalOnLowRisk`, `TestHighestRiskIntentFollowsTheRiskOrder`.
- remote: `TestRiskCeilingMapsEveryClassAndNothingElse`.
