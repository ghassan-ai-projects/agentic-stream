# Worker protocol

The current-v1 worker protocol is a Protobuf/gRPC contract for separate Go
EpisodeWorker processes. The source is
[`docs/design/contracts/runtime-v1.proto`](../../docs/design/contracts/runtime-v1.proto);
generated Go output is under
[`proto/agenticstream/runtime/v1/`](../../proto/agenticstream/runtime/v1/).

## Services

```text
EpisodeWorker.Handshake(HandshakeRequest) -> HandshakeResponse
EpisodeWorker.Execute(EpisodeRequest) -> stream EpisodeEvent
EvidenceTools.Call(EvidenceToolCall) -> EvidenceToolResult
```

## Compatibility requirements

A worker must declare compatible protocol/contract versions, a stable worker
identity, non-interactive execution, and requested features. The current
runtime validates the versions, identity, non-interactive flag, and requested
feature compatibility. Protocol fields for episode kinds and replay/shadow
capabilities exist, but their full semantic enforcement is not yet complete.

## Episode request

The request binds:

- episode, trigger, tenant, Situation ID/version, snapshot bytes/digest;
- Decision schema, tool catalog, intent catalog, skill references, and digests;
- objective/prompt and their digests;
- executor/model policy and finite budget;
- deadline, lane, kind, risk ceiling, allowed Intent types;
- evidence time range and capability token;
- attempt ID, fence, cancellation/supersession keys, trace context, and
  active/shadow dispatch policy. Some capability fields are protocol data whose
  runtime enforcement remains a release gap; do not infer support solely from
  their presence in the message.

## Episode events

Events are sequenced and carry episode/attempt/fence identity. They may report
start, model progress, tool lifecycle/progress, budget updates, diagnostics,
Decision proposals, cancellation, and one terminal result. The runtime rejects
late, wrong, incomplete, or budget-violating streams.

## Change workflow

```bash
make proto-generate
make proto-check
go test ./internal/executor/conformance ./internal/worker ./internal/episodes
```

Protocol changes require an ADR or accepted decision, compatibility analysis,
generated-stub review, and conformance evidence.

## Next reads

- [Worker boundary](../architecture/worker-boundary.md)
- [Build a Go worker](../guides/build-a-go-worker.md)
- [Compatibility](../overview/compatibility.md)
