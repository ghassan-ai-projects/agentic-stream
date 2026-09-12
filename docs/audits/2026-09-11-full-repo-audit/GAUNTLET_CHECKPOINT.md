# Gauntlet Checkpoint

Checkpoint: G-001 Round 1 A-049 accepted gain
Recorded: 2026-09-12 Europe/Berlin
State: active; Round 1 accepted and ready to advance

## Fixed goal, bar, and boundaries

- Goal: resolve every actionable finding in `INDEX.md`; no finding remains
  without a passing resolution and evidence.
- External bar: `GAUNTLET_BAR.md` is the authoritative release bar.
- Boundaries: highest-quality Go, minimal design-consistent changes, no new
  architecture/dependencies/network/secrets, preserve user changes, prove
  behavior with tests, commit after every round.
- Normalized scenes: `runtime-safety`, `durability`, `contract`, `quality`, and
  `artifact` as defined in `GAUNTLET_BAR.md`.

## Artifact receipts

Current artifact after Round 1:

- Audit source: `INDEX.md` plus all `A-*.md` files in this directory.
- Code baseline: commit `da7c5a1` (`fix(policy): enforce R3/R4 denial first; end approval re-pending loop (A-041)`).
- Already accepted audit fixes in history: A-058 `70168ef`, A-030 `c88529c`,
  A-013 `4499df2`, A-041 `da7c5a1`.
- Accepted gain: A-049 implementation and tests in `internal/ingress/jsonl.go`
  and `internal/ingress/jsonl_test.go`.
- Audit reports, `GAUNTLET_BAR.md`, and this checkpoint are included in the
  Round 1 artifact and are no longer treated as disposable working notes.
- Pre-commit source receipt: the exact staged content is the A-049 diff,
  `INDEX.md`, all `A-*.md` audit reports, and the Gauntlet artifacts listed
  above; post-commit receipt is recorded below after commit.
- Content hashes at the Round 1 judgment surface: `INDEX.md`
  `fba40784638cca237293ddd8f362465172dbe0f85e994f1fe881bd4f899dd211`,
  `GAUNTLET_BAR.md`
  `d4c314df617d0103879c816f31f932fb3f5978c666b1856f3dce60c6f6e79b70`,
  `internal/ingress/jsonl.go`
  `e3478ecd57affbd7740654c7845a654f629d2687cf1459bf6a9de607ad2e0832`,
  `internal/ingress/jsonl_test.go`
  `d0de82184f8af1e6e46014fdfbf328ee031849e73e1446eb02e6d5c075dcffbe`.

Best artifact receipt:

- Best accepted code artifact is commit `da7c5a1` plus the four prior accepted
  audit-fix commits listed above.
- Round 1 best candidate: the validated A-049 change plus the audit artifact;
  content commit receipt: `646132a` (`audit: capture full-repo gauntlet and
  close ingress findings`).

## Decomposition and dependency map

| Unit | Scope | Depends on | Ready Frontier |
|---|---|---|---|
| R1 ingress acceptance | A-049 current diff; audit artifact capture | baseline | ready |
| R2 durability/high-risk runtime | A-002, A-003, A-005, A-008, A-009, A-012, A-017, A-019, A-028, A-035, A-038, A-050, A-062 | R1 only for clean artifact state | queued |
| R3 contract reconciliation | A-092, A-095 plus affected compiler/operator behavior | R2 operator/spec decisions | queued |
| R4 dead/divergent production surfaces | A-001, A-004, A-006, A-007, A-011, A-014–A-024, A-026–A-047, A-051–A-069 | R2/R3 where APIs or schemas change | queued |
| R5 test-quality sweep | A-070–A-088 and tests added by R2–R4 | corresponding production behavior | queued |
| R6 whole-artifact judgment | every A-file, INDEX, bar, checkpoints, full gates | R1–R5 | queued |

Dependency policy: repair release-blocking semantics before deleting or
refactoring dependent surfaces; reconcile contracts before treating schema-only
changes as complete; keep test-only cleanup after behavior is stable.

## Critic isolation and quality lenses

Critics must inspect the current committed artifact and working tree without
editing the lead checkout or sharing conclusions with another critic. No critic
finding counts as approval until the lead reproduces it. Lenses:

- safety/correctness: invariants, fail-closed behavior, data integrity,
  cancellation, race and TOCTOU paths;
- architecture/durability: boundaries, transactions, leases, recovery,
  idempotency, deterministic replay, dead/divergent code;
- contracts/operators: schema, migrations, compiler/runtime parity, data-only
  domain behavior, wire compatibility;
- tests/quality: assertion strength, bounded synchronization, coverage value,
  duplication, exported-surface hygiene, and CI reproducibility;
- operator readiness: logs, telemetry, error truthfulness, resource bounds,
  and documented commands.

## Protected Gain Ledger

| Gain | Receipt | Protection |
|---|---|---|
| A-058 virtual timer ordering/reset cleanup | `70168ef` | do not regress interleaved due-time ordering or reintroduce dead Reset |
| A-030 lineage/state-write integrity | `c88529c` | preserve injective lineage IDs and zero-row detection |
| A-013 target/entity binding | `4499df2` | every target remains bound to trusted episode identity |
| A-041 risk/approval routing | `da7c5a1` | R3/R4 cannot enter approval loops or dispatch |
| A-049 connector-scoped quarantine/line bounds | current uncommitted diff | accept only with focused tests and preserve raw-input fail-closed behavior |

## Latest locked verdict and largest gap per active unit

- R1: `LOCKED PASS` — A-049 resolution is recorded; focused race tests pass;
  largest remaining gap is the full-repo gate and independent whole-artifact
  judgment after all rounds.
- R2: `PENDING` — largest gap is unverified cancellation/lease/epoch and
  operator-window behavior in the HIGH findings.
- R3: `PENDING` — largest gap is the storage-contract chimera and schema
  admitted-but-unimplemented surfaces.
- R4: `PENDING` — largest gap is the amount of dead/divergent production code;
  wire-or-delete decisions are not yet recorded.
- R5: `PENDING` — largest gap is false-confidence and missing failure-path
  tests.
- R6: `PENDING` — no whole-artifact judgment has been run after changes.

## Active causal probe

Closed: A-049 focused probe passed with
`env GOCACHE=/tmp/agentic-stream-go-cache go test -race ./internal/ingress ./internal/replay ./internal/runtime`.
The next active causal probe is the R2 high-risk transaction/operator suite.

## Cost consumed and remaining

- Consumed: baseline inspection plus one focused A-049 race validation; no
  explicit token budget supplied.
- Remaining: continue until the bar is met or a truthful external blocker is
  evidenced. Avoid repeated scans and retain only relevant context.

## Truthful state

The audit is not solved. Five HIGH findings are now accepted (four prior
commits plus A-049); the remaining high-risk runtime, operator, contract, and
test findings are still open. Round 1 validation passed, but the commit receipt
must be added immediately after the commit and no whole-artifact completion
claim is permitted.

## Next concrete artifact action

Commit the Round 1 artifact, record its exact commit receipt, then advance to
the R2 high-risk Ready Frontier with a bounded causal probe for transaction,
lease/fencing, operator-window, and native-provider cost behavior.

## Round 1 evidence receipt

- Focused validation: PASS — ingress, replay, and runtime packages under the
  race detector.
- Diff hygiene: PASS — `git diff --check` before commit.
- Resolution artifact: `A-049-ingress-jsonl.md` Resolution section and the
  `INDEX.md` `✅ FIXED` marker.
- Commit receipt: `646132a` (Round 1 content and artifact commit).

---

## Checkpoint: G-002 Round 2 high-risk runtime gains

Recorded: 2026-09-12 Europe/Berlin
State: active; five targeted findings have passed lead reproduction and are
ready for independent re-review and the repository-wide gates

### Fixed goal, bar, boundaries, and normalized scenes

- Goal unchanged: resolve every actionable finding in `INDEX.md`; no finding
  remains without a passing resolution and evidence.
- Bar unchanged: `GAUNTLET_BAR.md` remains authoritative; the round is not a
  whole-artifact pass until the remaining findings and the full gate are closed.
- Boundaries unchanged: highest-quality Go, minimal design-consistent changes,
  no new architecture/dependencies/network/secrets, preserve user changes,
  prove behavior with tests, and commit after every round.
- Normalized scenes unchanged: `runtime-safety`, `durability`, `contract`,
  `quality`, and `artifact`.

### Exact current and best artifact receipts

- Round entry HEAD: `413c519bc67d7244e7e91b0dba8889a6c59461cf`
  (`audit: record gauntlet round 1 receipt`).
- Current pre-commit judgment surface consists of the Round 2 source/test
  diff, five Round 2 finding reports, the five prior fixed-report verdict
  updates, `INDEX.md`, and this checkpoint. The worker-budget and epoch-cost
  hardening are included in the source candidate but remain subject to the
  next independent re-review.
- Current exact file receipts before the Round 2 content commit:

  | Artifact | SHA-256 |
  |---|---|
  | `INDEX.md` | `7a9f015a38f12505921b8ce6728fd635f23dab260ef581fbff19e283fc911406` |
  | `GAUNTLET_BAR.md` | `d4c314df617d0103879c816f31f932fb3f5978c666b1856f3dce60c6f6e79b70` |
  | `internal/actions/dispatcher.go` | `4db87d579261ebf85100643d9704b72d1f097a7146b17703aab493c72d02f6ac` |
  | `internal/actions/dispatcher_test.go` | `06bccce73cd729ac2443070d72fd7eb64f999b313a373ad33d214246fa307d3f` |
  | `internal/duration/duration.go` | `7508a58ea0170feb0ee1d23dddb59fff5e620f16b01552c3ef1ed886f8575ac2` |
  | `internal/episodes/assembler.go` | `903fc9536511100b08cb03b4a0694761ff749748c239265c14e55f3bb5cf5058` |
  | `internal/episodes/executor.go` | `f6efb2e4dbdfb09078f945b7d3d02d77a2f19edcc29854f81e042b25a9763de9` |
  | `internal/episodes/worker_executor.go` | `8c834f3bcaccc8157f1a3ca851c33d0281862cb858d9ae295f6594d2b19b0ef7` |
  | `internal/episodes/budget.go` | `8b264357e02bdb7a021926f8db1412493e808cb8a13ab0fcee0ca3beda7c8219` |
  | `internal/episodes/budget_test.go` | `a2a1e6993fa921bb037398d5ae20600c2083342e9eb88369c2152a8482f27a53` |
  | `internal/episodes/cancellation_test.go` | `4071ab28dcf6bc859b7f3cefed127ecf39a6ac270ded87ca597323f4b8d09fee` |
  | `internal/episodes/worker_executor_test.go` | `95ecf7f6b2a99f20d8eecc9b0e19d94adc1ad000f7cf1e5058789c709f5b0557` |
  | `internal/executor/native/native.go` | `12c79f379d8cc6d81bf140cf2751f24370ffdb74f0a7c28dfe57eb8d845538ad` |
  | `internal/executor/native/native_test.go` | `263e2e7e4a82fa78897511d21554f723908fd1e475000d172c159432cb93850e` |
  | `internal/executor/native/openai.go` | `a7c073c15bc3ce335a55403bc19f00e180bd5463de400f6e637435c2ca7f00f0` |
  | `internal/executor/native/openai_internal_test.go` | `dbc8d0b730a56d64d39cbbf69f271764da69fbbe36808e1cd4f5ba435eacb7ef` |
  | `internal/executor/native/openai_test.go` | `2315aa4748c36a498465b93f0fb3e8aa8cc8e8e29f5c36c3e88f65b8bf5a969d` |
  | `internal/policy/policy_evaluate.go` | `aeb55d4cd865a307dbb36c64035c0937a49b9b3d0ca8d0fe2b79001c3cdbce59` |
  | `internal/storage/epoch_control.go` | `33a89676bef755bbebcf3f701edb20f4cfe7e59867edbe9e713e453f6589df23` |
  | `internal/storage/epoch_control_test.go` | `35d8b58e0227af7c6dd9f4820d551b4abc8aae02d465582923a42da0cb41ca73` |
  | `internal/worker/budget.go` | `761f85fa8c94510d6cac888a601063f8a24017f6d5b6f2b913f071ba766abf12` |
  | `internal/worker/server.go` | `329f758f600c61b2925384248e8454b917a821da152f46591830e60d5c673015` |
  | `internal/worker/server_test.go` | `67e0556793571525263b1f5adc83bf606ae8380ab4efaaeb532b3b9ea53a1d0e` |

- Best accepted artifact remains the Round 1 receipt commit
  `413c519bc67d7244e7e91b0dba8889a6c59461cf` plus the earlier protected
  commits. The Round 2 progress commit requested by the user will preserve
  the current candidate for resumption, but does not upgrade the whole-audit
  verdict or close the independent-review gap.

### Current decomposition, dependency map, and Ready Frontier

| Unit | Scope | Depends on | Ready Frontier |
|---|---|---|---|
| R1 ingress acceptance | A-049 | baseline | `LOCKED PASS` |
| R2-A transaction/fencing | A-002, A-003, A-062 | R1 | candidate gains; re-review and full gates |
| R2-B provider budgets | A-012, A-038 | R2-A request/persistence semantics | candidate gains; re-review and full gates |
| R2-C remaining runtime | A-008, A-009, A-017, A-019, A-028, A-035, A-050 | R2-A/B where APIs are shared | next implementation frontier |
| R3 contract/operator reconciliation | A-005, A-092, A-095 plus affected compiler/operator behavior | R2-C decisions | queued after support matrix is fixed |
| R4 dead/divergent production surfaces | A-001, A-004, A-006, A-007, A-011, A-014–A-024, A-026–A-047, A-051–A-069 | R2/R3 API and contract decisions | queued |
| R5 test-quality sweep | A-070–A-088 and tests added by R2–R4 | corresponding production behavior | queued |
| R6 whole-artifact judgment | every A-file, `INDEX.md`, bar, checkpoints, and full gates | R1–R5 | queued |

Dependency policy remains: repair release-blocking semantics before deleting
dependent surfaces; reconcile contracts before closing schema-only findings;
then perform test-only cleanup against stable production behavior.

### Critic-isolation rule and quality lenses

Critics inspect the current artifact independently, do not edit the lead
checkout, and do not share conclusions. Their findings are challenges until
the lead reproduces them. The active lenses remain safety/correctness,
architecture/durability, contracts/operators, tests/quality, and
operator-readiness/resource bounds.

### Protected Gain Ledger

| Gain | Receipt | Protection |
|---|---|---|
| A-058 virtual timer ordering/reset cleanup | `70168ef` | preserve interleaved due-time ordering and the removed dead Reset surface |
| A-030 lineage/state-write integrity | `c88529c` | preserve injective lineage IDs and zero-row detection |
| A-013 target/entity binding | `4499df2` | every target remains bound to trusted episode identity |
| A-041 risk/approval routing | `da7c5a1` | R3/R4 cannot enter approval loops or dispatch |
| A-049 connector-scoped quarantine/line bounds | `646132a` | preserve raw-input fail-closed handling, checkpoint error truth, and bounded lines |
| A-002 expired-lease identity restoration | Round 2 content commit pending | reclaim must finalize unknown outcomes with tenant/intent identity and no effector call |
| A-003 cancellation-proof episode persistence/epoch quarantine | Round 2 content commit pending | produced/cancelled and late-kill paths must commit terminal state, never poison the queue |
| A-012 native provider budget/cost accounting | Round 2 content commit pending | timeout/cancel cost, finite ceiling, bounded retry, and canonical output errors remain visible |
| A-038 OpenAI HTTP/usage bounds | Round 2 content commit pending | provider calls cannot hang or silently settle unreported spend |
| A-062 atomic epoch kill/fail-closed state | Round 2 content commit pending | kill and supersession share a transaction; missing control fails closed |

### Latest locked verdict and single largest gap per active unit

- R1: `LOCKED PASS` — A-049 report/index resolution and focused race probe
  are committed; largest gap is the not-yet-complete whole-artifact judgment.
- R2-A: `CONDITIONAL PASS — PAUSED` — current full race and vet probes pass for
  A-002/A-003/A-062; largest gap is independent challenge of transaction
  snapshots, reservation release, and late-arriving worker behavior.
- R2-B: `CONDITIONAL PASS — PAUSED` — current full race probe passes for
  A-012/A-038 and worker budget hardening; largest gap is proving provider
  compatibility and usage omission behavior across the full native-worker
  path.
- R2-C: `PENDING` — largest gap is the remaining runtime findings, especially
  operator/schema no-ops and simulator data-only compliance.
- R3: `PENDING` — largest gap is the storage-contract chimera and admitted
  schema surface that the runtime does not implement.
- R4: `PENDING` — largest gap is the unresolved dead/divergent production
  surface inventory.
- R5: `PENDING` — largest gap is false-confidence and failure-path coverage.
- R6: `PENDING` — no whole-artifact judgment has passed.

### Active causal probe

- Lead probe completed: `env GOCACHE=/tmp/agentic-stream-go-cache go test -race
  -count=1 ./...` — PASS on the current working tree.
- `env GOCACHE=/tmp/agentic-stream-go-cache go vet ./...` — PASS.
- `git diff --check` — PASS.
- `make ci-check` had passed earlier with the pinned `protoc` 35.1 toolchain;
  the host's `protoc` 36.0 remains an environment mismatch. It was not rerun
  after this final paused candidate was assembled.
- Independent Round 2 re-review was not completed after the final hardening.
  On resume, re-open this committed artifact, regenerate the judgment surface,
  and replay the relevant worker/epoch/provider challenge before R2-C.

### Cost consumed and remaining

- Consumed: baseline and Round 1 artifact passes; three-lens challenge;
  Round 2 implementation; current full race, vet, and diff validation. No
  explicit token budget or machine-readable token meter was supplied.
- Remaining: the user requested a stop after this progress commit. On resume,
  continue from the committed Ready Frontier without resetting validation or
  plateau history.

### Current truthful state

The audit is not solved. Ten findings are marked `✅ FIXED` in the index (four
prior commits, A-049, and the five Round 2 targets); 67 findings remain open
and seven baseline HIGH findings remain open. The current Round 2 candidate
passes the full race and vet probes and is being committed as a resumable
progress artifact at the user's request. Independent re-review after the
latest hardening and the full pinned CI gate remain outstanding. No
whole-artifact completion claim is permitted. Work is intentionally stopped
after the receipt commit.

### Next concrete artifact action

On resume, inspect the real Round 2 progress commit and this checkpoint,
confirm the unchanged bar/boundaries, regenerate one judgment surface, replay
the worker/epoch/provider challenge, and then advance to the R2-C
operator/schema/runtime frontier only after the current candidate is accepted.

### Round 2 pause receipt

- User decision: commit the current progress and stop; do not claim audit
  completion.
- Current evidence: full `go test -race -count=1 ./...`, `go vet ./...`, and
  `git diff --check` pass on the candidate.
- Current content commit and post-commit checkpoint receipt are recorded below
  once written.
