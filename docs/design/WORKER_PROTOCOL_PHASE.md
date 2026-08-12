# Worker protocol phase status

This phase freezes and proves the current-version Go worker boundary. It is a
deliberately narrow vertical slice, not a claim that the full worker release
gate is complete.

Implemented:

- pinned `protoc`, Go protobuf/gRPC generators, committed Go stubs, and CI drift
  checking;
- exact current protocol and contract version handshake, with no legacy or
  N/N-1 behavior;
- immutable request validation, W3C trace-context validation, signed-fence
  identity checks, event sequence checks, terminal enforcement, bounded event
  count/bytes, and panic sanitization;
- a streamed `EpisodeWorker` adapter to the existing fenced episode executor;
- wall-time budget propagation and cancellation persistence using a detached
  bounded database context.

Explicitly unsupported in this phase:

- `EvidenceTools`, capability-token issuance or verification, and evidence
  access. Requests carrying an endpoint or token are rejected closed.
- UDS listeners, child-process supervision, remote workers, mTLS, Python
  fixtures, and cross-language compatibility fixtures.
- stale-attempt lease expiry and process-crash reissue; the existing fence
  model rejects stale output, while recovery policy is a subsequent phase.
- runtime-owned hard accounting for model calls, tokens, tool calls, result
  bytes, retries, and cost. Worker `BudgetUpdated` telemetry is not an
  authorization source.

Those surfaces are subsequent phases. Until they are implemented and tested,
the completion bar's worker conformance gate remains open.
