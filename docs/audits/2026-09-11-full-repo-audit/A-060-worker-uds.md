# A-060 · `internal/worker/uds.go`

LOC: 159 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported dial helper has a production caller in this repo, or is removed.
- The absolute-clean-Unix-path rule is implemented once across the repo.
- `Close` is idempotent like its ingress counterpart.

## Findings
- **MED F1. `DialEvidenceSocket` and `DialEpisodeWorkerSocket` have no production callers** — `internal/worker/uds.go:83-96,101-103`. Repo-wide grep: `DialEvidenceSocket` is called only from tests (`internal/evidence/uds_integration_test.go:38`, `internal/worker/uds_test.go:60`); `DialEpisodeWorkerSocket` only from the conformance test (`internal/executor/conformance/conformance_test.go:116`) and is a three-line pass-through to the TLS variant. The runtime itself dials only via `DialEpisodeWorkerSocketTLS` (worker_runtime.go:178), and the evidence socket is dialed by the out-of-process worker, not by Go production code. These are exported symbols in an `internal/` package serving as test scaffolding. Fix: move both into the respective `_test.go` files or behind an exported test-helper, keeping only the TLS dial in production code.
- **MED F2. Socket-path validation rule duplicated across packages** — `internal/worker/uds.go:154-159` vs `internal/ingress/live_socket.go:358-363`. Identical rule (absolute, no NUL, no `://`, `filepath.Clean` equality), different error text; a future rule change (e.g. rejecting `..` segments explicitly) must be made twice. Fix: single shared implementation in one low-level package, both callers wrapped with their own message.
- **LOW F3. `cleanListener.Close` is not once-guarded** — `internal/worker/uds.go:131-150`. Unlike ingress's `cleanLiveListener` (live_socket.go:406-423, `sync.Once`), a second `Close` re-runs stat/remove and returns `close evidence listener: ...` after the first close already succeeded. Current callers close once, but the asymmetry invites double-close regressions. Fix: guard with `sync.Once` and cache the result, matching the ingress listener.

## Checked, not an issue
- P1: errors wrapped `%w`; listener cleanup closes and removes the socket path on chmod/stat failure; probe dials time-bounded (100ms) with context cancel.
- P2: workers reachable only through governed paths — private parent directory enforced (0700, symlink and non-dir refused), stale/active socket refused rather than unlinked, socket chmod 0600, symlink-swap protection via `os.SameFile`; dialers pinned to `unix` with `passthrough:///` targets and no remote trust (insecure credentials documented as capability-gated).
- P5: exported symbols documented with their transport rationale.
- P6: uds_test.go covers privacy/cleanup and Unix-only dialing.
