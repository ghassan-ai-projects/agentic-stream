# DUP-022: cmd re-derives rules and shapes owned by device, watch and replay

- Status: open
- Severity: medium
- Verdict (finders): REAL
- Themes: business rules, contracts and shapes
- Wave: 1
- Finder sources: R7, S12 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

The physical-actuation consent rule must exist once in `device`; cmd keeps only flag-presence checks. Move the `watch_id` projection to watch and the shadow report projection to replay. Error strings and `--json` field names are pinned by cmd tests.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R7: Effect-profile (physical actuation) authorization rule restated in cmd

- Verdict: REAL (business decision in cmd duplicating the owning module)
- Shared meaning: a physical effect profile requires explicit live-actuation and owner authorization; simulated must not configure a gateway; emulator/physical need a gateway link and cannot be combined with replay.
- Sites:
  - internal/device/internal/domain/profile.go:33-85 - `CheckEffectProfile` (`requireNoGateway`, `requireLiveGateway`, `requireActuationConsent`: texts "physical effect profile requires explicit live actuation", "physical effect profile requires owner authorization"), exposed as `device.ValidateEffectProfile` (internal/device/profile.go:29).
  - cmd/agentic-stream/effect_profile.go:50-66 - `validate` switch per profile; :53-56 "simulated effect profile cannot configure device gateway options"; :81-91 `validatePhysicalAuthorization` returns the SAME two texts for the live-actuation and owner checks; :68-79 gateway option requirements.
  - cmd/agentic-stream/effect_profile.go:146-157 - `validateGatewayLink` calls `device.ValidateEffectProfile` again with the real link (so the physical rule is evaluated twice per start-up).
- How they differ: cmd validates before connecting (so it needs flag-level checks), device validates after the link exists. Both enforce the physical consent rule with identical messages; the cmd copy of the replay rule differs slightly (`ReplaySource` delegation at :59-60 only).
- Risk if left: hardening the physical rule (e.g. require a second consent flag) in device leaves the pre-connect check in cmd accepting it, or vice versa; safety-critical rule in two layers.
- Proposed canonical owner: `internal/device` (already exposes `ValidateEffectProfile`; cmd already imports device).
- Proposed fix: let `EffectProfileConfig` carry `HasGatewayLink bool` (or reuse `GatewayLink != nil` with a pre-connect marker) so cmd can call `device.ValidateEffectProfile` before dialing with the flag values, then delete `validatePhysicalAuthorization` and the simulated/gateway-combination switch arms; keep only the flag-presence checks (socket, catalog, firmware digest) that are CLI concerns.
- Behaviour to preserve: the exact error strings (effect_profile_test.go in cmd), flag names, fail-closed ordering (validation before any device connection).
- Verification: cmd/agentic-stream/effect_profile_test.go, device profile tests. New: cmd test asserting the pre-connect and post-connect checks produce the device-owned message verbatim.

### Finder report S12: cmd/agentic-stream re-derives shapes owned by other modules

- Verdict: REAL (small)
- Shared meaning: the result of a `watch` effect (`watch_id`) and the replay shadow report.
- Sites:
  - internal/watch/internal/app/install.go:45 - writes `ProviderResult: {"accepted": true, "watch_id": watchID}`
  - cmd/agentic-stream/intent_command.go:80-90 - `reportedWatchID` re-declares `struct{ WatchID string \`json:"watch_id"\` }` and unmarshals `OutcomeView.ProviderResult`
  - internal/replay/internal/domain/result.go:10-25 - `Result`/`Finding`/`ShadowComparisonResult`; cmd/agentic-stream/run_shadow.go:48-100 - `shadowReport`/`shadowComparison`/`shadowFinding` are a field-for-field JSON re-projection (renaming `TamozDecisionJSON` to `candidate_decision`), plus `differingComparisons` (run_shadow.go:36-43) counting `!DecisionsEqual`, a rule about the replay result that lives in cmd
- How they differ: no divergence yet; the JSON contract of the shadow report and of the watch install result is defined in cmd (and implicit in watch) rather than by the owner.
- Risk if left: renaming `watch_id` in watch makes `intent` inspection silently drop the installed watch; `run-shadow --json` changes only when someone remembers to edit cmd.
- Proposed canonical owner: `internal/watch` exposes `watch.InstalledWatchID(providerResult []byte) string` (watch already owns the writer); `internal/replay` exposes `Result.DifferingComparisons()` and a `Report` JSON view (replay is composed by cmd; cmd already imports it).
- Proposed fix: move the two projections to their owners; cmd calls them. cmd/agentic-stream then has no `json:"..."` tags of its own for these.
- Behaviour to preserve: `run-shadow --json` field names and order (`experiment_shadow_test.go` pins them) and `intent` text/JSON output.
- Verification: cmd experiment tests and `intent_command` tests; add an owner-side test round-tripping the watch result.

## Outcome

Not started.
