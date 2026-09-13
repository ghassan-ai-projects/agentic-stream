# A-026 · `internal/worker/server.go`

LOC: 314 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Handler panics leave a server-side log record, not only a wire error.
- Version-compatibility helpers state what they actually compare.
- No redundant conditions in the event validator.

## Findings
- **LOW F1. Panic recovery discards the panic value and stack without logging** — `internal/worker/server.go:113-117`. `recover()` is converted to `codes.Internal "worker handler panic"` and dropped; nothing is written via slog, so a worker-handler panic is invisible in server logs and undiagnosable after the fact. Fix: capture `panicValue := recover()`, `slog.Error("worker handler panic", "panic", panicValue, "stack", string(debug.Stack()))`, then return the wire error.
- **LOW F2. `sameMajor` performs exact equality, not major-version comparison** — `internal/worker/server.go:208,253-255`. The function compares `strings.TrimSpace(got) == strings.TrimSpace(want)` — identical semantics to the `!=` used in `Handshake` (:80-81) under a name that promises semver-major logic. Either the name lies or the check is weaker than intended; both call sites want exact-match `1.0`. Fix: rename to `sameVersion` (or delete and use `==` consistently with Handshake).
- **LOW F3. Redundant zero-sequence condition in the validator** — `internal/worker/server.go:290`. `event.GetSequence() == 0 || event.GetSequence() != v.nextSequence+1` — the first disjunct is implied by the second because `nextSequence+1 >= 1`. Drop the first clause.

## Checked, not an issue
- P1: stream validator state mutated only under its mutex; deadline and wall-time budget derived from the stream context with deferred cancels; post-success `ctx.Err()` check catches deadline races (155-157); send errors wrapped.
- P2: untrusted worker output never reaches the runtime unvalidated — identity (episode/attempt/fence) enforced on every event, strict sequence gaps rejected, decision digest length and identity checked, terminal-only-once, event/request/stream byte and count ceilings enforced before `stream.Send`; evidence endpoint constrained to a private clean UDS path; request requires non-interactive handshake and matching protocol/contract versions.
- P3: limits default in one place (`limits`); no speculative abstraction; exported constants (`ProtocolVersion`, `ContractVersion`, `EvidenceToolsFeature`, `Default*`) used by episodes/worker_executor.go.
- P5: gRPC status construction isolated in `wireError`/`wireErrorf` with deliberate wrapcheck rationale; handler stays thin.
- P6: server_test.go covers handshake, execute happy path, sequence/terminal/trace rejections.
