# DUP-027: Nil-clock defaulting and wall-clock reads that bypass the injected clock

- Status: partly fixed
- Severity: low
- Verdict (finders): REAL
- Themes: mechanisms
- Wave: 2
- Finder sources: M9 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Use `sources.OrPhysical`; fix the two wall-clock reads (runtime heartbeat, `TransitionAttempt`) only if they affect determinism.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M9: Nil-clock defaulting and wall-clock reads that bypass the injected clock

- Verdict: REAL (defaulting), DIVERGED (bypasses)
- Shared meaning: a component uses its configured `Now` (or `sources.Clock`) and falls back to the physical UTC clock; replay uses a virtual clock.
- Sites (defaulting written by hand instead of `sources.OrPhysical`): control/control.go:29-34 `utcNow`; evidence/internal/app/server.go:57-62 `Server.now`; evidence/internal/app/service.go:133-138 `IssueTime`; evidence/internal/app/ledger.go:38-44 `Ledger.now`; evidence/internal/app/capability.go:26-30 (`Issuer.Issue`); evidence/internal/app/capability_verify.go:39-44 (`checkValidity`); runtime/internal/store/recovery.go:56-62 `recoveryTime` (and :45 falls back to the owner's claim time when `Now == nil`); api/internal/transport/sse.go:46-48 (`cfg.Now = time.Now`, not UTC).
- Sites (reads physical time although a clock is injected): runtime/internal/app/service.go:92 `ReclaimExpired(ctx, time.Now().UTC())` in the owner heartbeat; episodeledger/operations.go:33 passes `time.Now().UTC()` as the identity-check clock to `TransitionAttempt`; spec/internal/store/deployments.go:59; executor/native/internal/store/evidence_tool.go:40; runtime/internal/transport/worker.go:180.
- How they differ: two clock shapes coexist (`sources.Clock` interface vs `func() time.Time` fields on evidence, control, runtime recovery, api); defaults differ (UTC vs local `time.Now`); the lease heartbeat reclaims evidence leases at wall time while the same ledger fences using its configured `Now`.
- Risk if left: virtual-clock/replay runs mix clocks in lease reclamation; a new component repeats the nil check once more.
- Proposed canonical owner: `internal/sources`: add `sources.NowFunc(now func() time.Time) func() time.Time` (nil -> physical, always UTC), or migrate the `Now func` fields to `sources.Clock`. evidence app already imports sources (allowedImports); control/internal/app would need `internal/sources` (new edge) or keep the single `utcNow` and let evidence call control's? (not allowed); prefer the sources helper.
- Proposed fix: introduce the helper, delete the six local copies, and thread the configured clock into the heartbeat reclaim and the identity-check clock where tests permit.
- Behaviour to preserve: UTC in all results; wall clock for `TransitionAttempt` identity check if that is deliberate (operations.go comment says "at the wall clock") - confirm with the author before changing.
- Verification: evidence ledger and capability tests, runtime recovery tests. New test: ledger reclaim with a virtual clock past `lease_until` interrupts the call without sleeping.

## Outcome

Verified (all sites re-read):
- Confirmed: seven hand-written nil-clock defaults (control `utcNow`, evidence `Server.now`, `Service.IssueTime`, `Ledger.now`, `Issuer.Issue`, `Verifier.checkValidity`, runtime `recoveryTime`) and the api SSE default `cfg.Now = time.Now`. All produced UTC, except SSE which applied `.UTC()` at use, so the SSE default was equivalent.
- Partly right: `recovery.go` also falls back to the owner's claim time when `Now == nil`. That is deliberate (recovery stamps rows with the claim transaction's time) and kept.
- Wrong or not a determinism issue: the runtime heartbeat `ReclaimExpired(ctx, time.Now().UTC())` and `episodeledger.TransitionAttempt`'s identity-check clock. Live composition (`cmd/agentic-stream/live.go`) configures the evidence ledger and owner with no `Now`, so both are physical there; the heartbeat is a wall-time ticker and the doc comment on `TransitionAttempt` says the owner-lease check is deliberately at the wall clock. Replay never runs the heartbeat. `spec` deployments, `executor/native` evidence_tool, `runtime/transport/worker.go` and `workerfake` reads are live-only or fixture code and were left alone.

Changed:
- `internal/sources/sources.go`: new `NowUTC(now func() time.Time) time.Time` (nil -> physical, always UTC) and `NowFunc(now)` (function form for components that store a `func() time.Time`).
- Deleted the copies: `internal/control/control.go` (`utcNow`; `owner.go` and `epoch.go` call `sources.NowFunc`), `internal/evidence/internal/app/{server,service,ledger,capability,capability_verify}.go`, `internal/runtime/internal/store/recovery.go` (`recoveryTime`), `internal/api/internal/transport/sse.go` (default block; read via `sources.NowUTC`).
- `architecture_test.go` allowedImports: added `internal/control -> internal/sources` and `internal/api/internal/transport -> internal/sources` (the evidence app and runtime store already imported sources).

Decisions: the helper lives in `sources` as the finder proposed (control's own copy could not be shared with evidence). The `Now func() time.Time` fields stay (migrating them to `sources.Clock` would change public configs for no determinism gain).

Deferred: threading the configured clock into the heartbeat reclaim and the `TransitionAttempt` identity check. Both are wall-clock by design in live runs and `ReclaimExpired` takes an explicit instant (signature change, no current need).

Pinned by: `TestNowUTCReadsTheConfiguredClockInUTCOrThePhysicalClock` (internal/sources), `TestLedgerReclaimsWithTheConfiguredVirtualClockWithoutSleeping` (internal/evidence/internal/app), plus the existing evidence, control, runtime recovery and SSE suites (unchanged, passing).

Note: while this ran, other fixers' timestamp-format work made `internal/evidence/internal/wire` tests and `runtime/internal/store` `TestRecoveryWithoutOverrideUsesClaimTimestamp` fail on `.000000000Z` vs `Z` text; those files are not touched by this change.
