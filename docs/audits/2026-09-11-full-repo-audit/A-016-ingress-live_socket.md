# A-016 · `internal/ingress/live_socket.go`

LOC: 423 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported method on `LiveUDSSource` has at least one production caller.
- Quarantine `reason_code` accurately reflects the rejection cause.
- The absolute-clean-Unix-path rule exists in exactly one package.

## Findings
- **MED F1. `WithQueueSize` and `WithClock` are dead exported methods** — `internal/ingress/live_socket.go:75-81,98-105`. Repo-wide grep finds zero callers of either (production and tests); the only option used outside the constructor is `WithTelemetry` (pipeline.go:299), and `WithLogger` is used solely by this package's own tests to silence output. Fix: delete `WithQueueSize` and `WithClock`; inline the queue-size default; demote `WithLogger` to an unexported test seam or keep with a test-only comment.
- **LOW F2. Connection read errors quarantined as `line_too_large`** — `internal/ingress/live_socket.go:287-289` with `:264-285`. `readLiveLine` returns both the intentional `errLiveSocketLineTooLarge` and wrapped transport errors (`read live ingress line: %w` at :283), but `processLine` labels every `item.readErr` `line_too_large` when calling `rejectRaw`. A connection reset or decode-side IO failure is misclassified in the durable quarantine record. Fix: branch on `errors.Is(item.readErr, errLiveSocketLineTooLarge)` and pass a distinct reason otherwise.
- **LOW F3. Socket-path validation rule duplicated across packages** — `internal/ingress/live_socket.go:358-363` vs `internal/worker/uds.go:154-159`. `validateLiveSocketPath` and `ValidateEvidenceSocketPath` implement the identical rule (absolute, no NUL, no `://`, `filepath.Clean` equality) with different error strings; the two copies can drift. Fix: move the rule into one shared low-level package (e.g. contractsv1 or a small transport helper) and have both call it.

## Checked, not an issue
- P1: sink processed serially line-by-line preserving per-partition ordering; bounded queue (128) and 16-client cap with overflow rejection logged; shutdown cancels, closes listener and clients, and joins all goroutines (152-159); canceled context treated as normal shutdown (183-185).
- P2: three-stage validation (JSON parse, `ValidateEnvelope`, schema `ValidateEnvelope`) before the sink sees data; malformed input quarantined with stable identity `live-uds:<instance>:<conn>:<line>`, never dropped silently; oversized frames keep only a bounded prefix; listener refuses symlinks/active sockets, chmods 0600, and cleans up via `os.SameFile` guard.
- P4: transport concerns stay here; the sink callback (pipeline.go:300-317) owns appending and pipeline advancement.
- P5: log/slog with structured reason codes; context first param; exported symbols documented.
- P6: live_socket_test.go covers quarantine, line bounds, reconnects, and sink-deadline propagation.
