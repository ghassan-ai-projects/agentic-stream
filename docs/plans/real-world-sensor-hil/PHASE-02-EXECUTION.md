# Phase 02 execution plan — paired supervisory shadow

Status: plan review required; implementation has not started.

This is the concrete execution plan for Phase 02 / G3 in
`02-shadow-path.md`. It is deliberately limited to the Agentic Stream shadow
boundary. It does not add a live model credential, a serial transport, a
physical effector, or a new decision engine in the event hot path.

## Bar

Phase 02 is complete only when all of these are true:

1. A deterministic baseline and the Tamoz shadow executor receive the same
   canonical, immutable Situation snapshot bytes and the same trusted
   `situation_id`, version, episode, policy, and spec identities.
2. Both outputs are schema- and digest-validated Decisions. Their intents are
   validated against the compiled catalog; an empty-intent abstention is a
   valid, scoreable decision.
3. One durable comparison record contains the snapshot digest, both executor
   identities and manifests, both decision digests/content, the aligned
   Situation identity, and the deterministic difference result.
4. Shadow execution cannot insert into `intents`, `commands`, or `outbox`, and
   cannot obtain a credential or invoke an effector. This is enforced by the
   shadow orchestration boundary and a test that inspects all effect tables.
5. Incomplete, adversarial, malformed, out-of-catalog, and stale outputs fail
   closed. Neither executor is promoted by the comparison path.
6. The comparison is reproducible from the same replay database/spec/trace and
   reports byte/content differences without depending on wall-clock time.

The following are not Phase 02 evidence: a fixture that merely returns a
manifest, a worker invocation count, a no-op effector, or a green unit test
that does not inspect durable effect tables.

## Current gap confirmed before implementation

The existing replay shadow capability accepts one report-only executor and
compares only a manifest against an optional recorded ledger. The existing
runtime shadow path scores a validated worker Decision, but it deliberately
does not persist `intents` or create commands. Neither path yet provides the
paired baseline/Tamoz comparison artifact required by this bar. The existing
decision validator also rejects an empty `intents` array, so abstention needs a
small contract change before the baseline can represent it.

## Design and ownership

- `internal/replay` owns effect-safe paired orchestration for replay trials.
- The deterministic baseline is a narrow, deterministic executor over the
  already-persisted snapshot. It may abstain; it may not call a model or
  action plane.
- Tamoz remains an external worker behind the existing typed worker/shadow
  boundary. Agentic Stream supplies the immutable input and validates its
  returned Decision; it does not embed Tamoz or provider SDKs.
- `internal/storage` owns one new append-only comparison record/table. It is
  not an action-plane ledger and must not be read as approval or promotion.
- Existing live `episodes` shadow scoring remains report-only. The new replay
  comparison must not weaken its independent `dispatchPolicy: shadow` guard.

## Implementation sequence

### 2.1 Freeze the decision contract for abstention

- Update the Decision schema and validator to allow `intents: []` while
  preserving all identity, snapshot, digest, and freshness checks.
- Add an optional `decision_type` enum with `need_more_evidence` as the
  abstention value. Require `decision_type: need_more_evidence` exactly when
  `intents` is empty; leave existing non-empty Decisions backward-compatible.
  This makes abstention explicit without introducing a free-form executable
  field.
- Add validator tests for valid abstention, missing `decision_type`, adversarial text,
  and out-of-catalog intent rejection. Update only the contract goldens that
  intentionally change.

### 2.2 Add paired shadow types and durable comparison evidence

- Extend the replay shadow input with trusted Situation/episode identity,
  snapshot digest, policy/spec revisions, and a defensive copy of canonical
  snapshot bytes.
- Add explicit baseline and Tamoz shadow executor capabilities. Keep the
  interfaces report-only and free of credentials/effectors.
- Add a migration and storage helper for an append-only `shadow_comparisons`
  record. Store canonical validated decisions and digests, executor versions,
  manifests, snapshot/policy/spec identities, and a deterministic comparison
  result. Reject duplicate comparison keys rather than overwriting evidence.
- Validate the comparison record before writing it and make its digest stable
  under map/key ordering.

### 2.3 Implement paired replay orchestration

- Require both executors for `ModeShadow`.
- Load and validate each Situation snapshot once; pass independent byte copies
  of the same bytes to both executors.
- Validate both executor outputs against the trusted replay identity, decision
  schema, intent catalog, and canonical digest. Treat malformed or mismatched
  output as a failed shadow trial, not as a promotion or partial comparison.
- Compare canonical Decision/Intent content and manifests deterministically;
  record differences as findings and persist the comparison artifact.
- Preserve the existing recorded-ledger compatibility only where it remains
  explicitly report-only; do not silently reinterpret an old manifest as a
  Decision.

### 2.4 Prove the safety and fairness properties

Add focused tests for:

- baseline/Tamoz byte-identical snapshots, aligned identity, and independent
  input buffers;
- deterministic abstention and incomplete-evidence abstention;
- adversarial evidence text remaining data, with an out-of-catalog intent
  rejected by the normal catalog validator;
- malformed, stale, wrong-digest, and wrong-Situation executor outputs;
- durable comparison content and reproducibility across two fresh runs;
- zero rows in `intents`, `commands`, and `outbox`, with no effector/credential
  capability available to either executor;
- existing runtime `dispatchPolicy: shadow` still recording its report-only
  result without entering policy/action governance.

## Validation and evidence

Before committing Phase 02, run the focused changed packages, `go vet ./...`,
`go build ./...`, `git diff --check`, and the repository documentation check.
Run `go test ./...` and `make ci-check`; if the environment still has the
known socket or protoc limitation, record the exact failing gate and do not
call G3 green. The phase commit message must name the durable comparison test
and the no-effects test.

## Explicit non-goals

- No real provider/model credential or provider SDK.
- No serial/device protocol or physical-profile wiring.
- No typed physical reconciliation; that is Phase 04.
- No promotion, automatic winner selection, or policy change based on the
  comparison. The Research Lab oracle remains the comparison authority.
