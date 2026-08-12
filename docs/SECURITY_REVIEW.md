# Security review checklist

Status: implementation controls are covered by tests; deployment approval
still requires the environment rehearsal record in
[runtime operations](runbooks/runtime-operations.md).

| Boundary | Control | Evidence |
|---|---|---|
| Raw event to cognition | Schema validation, canonical envelopes, quarantine | `internal/eventlog`, `internal/ingress` tests |
| Worker to evidence | Per-attempt scoped capability, fence, private UDS, bounded rows/bytes | `internal/evidence`, `internal/worker` tests |
| Worker to actions | Workers return Decisions only; policy revalidates intents | `internal/decisions`, `internal/policy`, `internal/actions` tests |
| Subscriber to notifications | Separate authorization hook, event allowlist, cursor audit, bounded lag | `internal/notify/sse_test.go` |
| Replay to production | Replay constructors exclude credentials/effectors | `internal/replay` tests |
| Secrets | API key supplied through process environment, never CLI output | `cmd/agentic-stream` wiring and runbook |
| Retained artifacts | Oversized evidence is referenced by digest and bounded store contract | `internal/executor/native` tests |

No Python worker, legacy protocol compatibility, arbitrary shell tool, or direct
model-to-effector path is part of the release boundary.
