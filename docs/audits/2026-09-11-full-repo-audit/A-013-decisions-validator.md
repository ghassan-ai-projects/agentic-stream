# A-013 · `internal/decisions/validator.go`

LOC: 510 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Every identity-bearing intent parameter is bound to the dispatched episode's trusted identity, regardless of which other parameters are present.
- Expiry checks fail closed when the timestamp cannot be parsed.

## Findings
- **[HIGH] F1. `parameters.target` bypasses entity binding when `entity_id` is absent** — `internal/decisions/validator.go:364-369`. The target check only runs when BOTH `parameters.target` and `parameters.entity_id` are present (`if entityID, ok := parameters["entity_id"]; ok && target != entityID`). A proposal carrying `target` without `entity_id` skips binding entirely, and the policy plane prefers `target` as the effector's `normalized_target` (`internal/policy/policy_helpers.go:34-48`) and embeds `parameters` verbatim into the command payload (`internal/policy/policy_command.go:97-103`). The protection the comment at lines 345-347 describes — identity parameters bound to the dispatched episode — therefore has a one-field hole: the model can steer an effect at an arbitrary 256-char target string that was never verified against `input.EntityID`. This depends on per-catalog schema goodwill, which is data, not an invariant. Fix: when `parameters.target` is present, require it to equal `input.EntityID` exactly as `entity_id` is checked (or require `entity_id` to be present whenever `target` is).
- **[LOW] F2. `isExpired` fails open on an unparseable `valid_until`** — `internal/decisions/validator.go:507-510`. A parse error yields `false` (not expired). Currently unreachable because `decision-v1.json:22` enforces `format: date-time` with `AssertFormat`, but as defense-in-depth for a security check the direction is wrong: an unparseable trusted-side timestamp should reject, not pass. Fix: return rejection on parse error.

## Resolution (2026-09-11) — FIXED
- **F1 (HIGH)** fixed: `parameters.target`, when present, now binds directly to `input.EntityID` (fail-closed when the validator has no entity identity), exactly as `entity_id` is bound — closing the target-without-entity_id hole. Added `TestValidateBindsTargetToEpisodeIdentity` covering mismatched target, missing episode identity, and the bound-target accept path.
- **F2** fixed: `isExpired` now returns `true` (expired → reject) on an unparseable `valid_until`, so the security check fails closed as defense-in-depth.
- Verified: `go build ./...` and `go test ./internal/decisions/` pass.

## Checked, not an issue
- P1: errors are structured rejections (`ValidationError`), nothing swallowed silently; no I/O, no races (stateless functions).
- P2: otherwise exemplary — digest verify, episode/attempt/fence/snapshot binding, risk-label equality (not clamping), risk ceiling fail-closed, per-intent schema, preset byte-equality, evidence grounding, at-most-one-actionable, kind-scoped compensation bypass, `denyNetworkLoader` blocks external `$ref` loads, catalog compiles fail-closed.
- P3: helpers (`toStringSlice`, `intValue`, `riskRank`, `reject`) all used; `CompileIntentCatalog` consumed by `internal/episodes/executor.go:568` and `internal/replay/replay.go:249`.
- P4: validator is a pure function; no storage, no policy overlap; "entity_id"/"target"/"compensates" are protocol-level conventions, not per-domain branches.
- P5: canonical JSON (RFC 8785) for every digest; exported symbols documented.
- P6: `validator_test.go` covers fail-closed catalog compilation and the core rejections; `go test ./internal/decisions/` passes.
- P7: digests deterministic; validation pure given `Input`.
