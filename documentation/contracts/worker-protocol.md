# Worker protocol

The v1 worker protocol uses Protobuf and gRPC to communicate with a separate
Go EpisodeWorker process. The source is
[`proto/agenticstream/runtime/v1/runtime-v1.proto`](../../proto/agenticstream/runtime/v1/runtime-v1.proto);
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
feature compatibility. The protocol also declares episode kinds and replay/shadow capabilities, but
the runtime does not yet fully enforce what all those fields promise.

## Episode request

The request binds:

- episode, trigger, tenant, Situation ID/version, snapshot bytes/digest;
- Decision schema, tool catalog, intent catalog, skill references, and digests;
- objective/prompt and their digests;
- executor/model policy and finite budget;
- deadline, lane, kind, risk ceiling, allowed Intent types;
- evidence time range and capability token;
- attempt ID, fence, cancellation/supersession keys, trace context, and
  active/shadow dispatch policy. Some capability fields are present in the message
  but are not fully enforced. Check current runtime support before relying
  on them.

## Episode events

Events are sequenced and carry episode/attempt/fence identity. They may report
start, model progress, tool lifecycle/progress, budget updates, diagnostics,
Decision proposals, cancellation, and one terminal result. The runtime rejects
late, wrong, incomplete, or budget-violating streams.

## Change workflow

```bash
make proto-generate
make proto-check
go test ./internal/testsupport/executorconformance ./internal/worker ./internal/episodes
```

Protocol changes require an ADR or accepted decision, compatibility analysis,
generated-stub review, and conformance evidence.

## Next reads

- [Worker boundary](../architecture/worker-boundary.md)
- [Build a Go worker](../guides/build-a-go-worker.md)
- [Compatibility](../overview/compatibility.md)
